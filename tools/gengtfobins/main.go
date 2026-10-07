// gengtfobins builds pkg/rules/gtfobins/table.go from a GTFOBins checkout
// (https://github.com/GTFOBins/GTFOBins.github.io).
//
// For each executable it keeps the functions that work when run with elevated privileges (the
// "sudo" context) and, for those that run code, how the recipe triggers it: the options and
// subcommands it passes, or nothing when the program is interactive (vi, less, ftp). Aliases and
// "inherit" entries are followed (vim inherits vi). Inheriting from a pager is recorded as Pager.
//
//	git clone --depth 1 https://github.com/GTFOBins/GTFOBins.github.io /tmp/gtfobins
//	go run ./tools/gengtfobins -src /tmp/gtfobins -out pkg/rules/gtfobins/table.go
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"log"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

type entry struct {
	Code     string         `yaml:"code"`
	From     string         `yaml:"from"`
	Contexts map[string]any `yaml:"contexts"`
}

type file struct {
	Alias     string             `yaml:"alias"`
	Functions map[string][]entry `yaml:"functions"`
}

// funcs maps GTFOBins function names to the Go constants in package gtfobins.
var funcs = map[string]string{
	"shell":                "Shell",
	"command":              "Command",
	"reverse-shell":        "ReverseShell",
	"bind-shell":           "BindShell",
	"file-write":           "FileWrite",
	"file-read":            "FileRead",
	"library-load":         "LibraryLoad",
	"upload":               "Upload",
	"download":             "Download",
	"privilege-escalation": "PrivilegeEscalation",
}

// runsCode are the functions whose recipes are scanned for triggers.
var runsCode = map[string]bool{"shell": true, "command": true, "reverse-shell": true, "bind-shell": true, "library-load": true}

// pagers are inherit sources recorded as Pager.
var pagers = map[string]bool{"less": true, "more": true, "man": true, "pg": true}

// generic options that appear in recipes but don't cause the escape.
var generic = map[string]bool{"-n": true, "-q": true, "-y": true, "-v": true, "-u": true, "-it": true, "--rm": true, "-a": true, "-nx": true, "-Q": true, "-nw": true}

// noise are recipe words that set up a multi-step recipe or pick a mode, but are also how the
// program is normally used, so matching them would flag ordinary commands.
var noise = map[string][]string{
	"systemctl": {"--now", "enable"},
	"git":       {"--allow-empty", "-m", "init", "-C"},
	"tar":       {"cf", "xf", "--checkpoint"},
	"find":      {"-quit"},
	"pip":       {"--break-system-packages"},
	"apt-get":   {"update"},
	"apt":       {"update"},
	"less":      {"-c"},
	"watch":     {"-c"},
	"ssh":       {"-o"},
}

// interactive lists programs with a shell escape in their own interface (vi's :!sh, gdb's !sh)
// whose GTFOBins recipes only show the non-interactive form.
var interactive = map[string]bool{"vi": true, "emacs": true, "ed": true, "gdb": true, "mysql": true, "psql": true, "sqlite3": true, "ftp": true, "sftp": true, "mail": true, "mutt": true, "irb": true}

// notArgs are programs whose recipes pass code as an operand but whose normal operands are not
// code (sed's script only runs commands with the e command, which is a trigger).
var notArgs = map[string]bool{"sed": true}

// payload marks recipe text that is the code being run, as opposed to an ordinary operand.
var payload = []string{"/bin/sh", "/path/to/command", "/path/to/lib", "/path/to/temp-file", "exec ", "system("}

type result struct {
	funcs       []string
	interactive bool
	args        bool
	triggers    []string
}

func main() {
	src := flag.String("src", "", "GTFOBins checkout")
	out := flag.String("out", "pkg/rules/gtfobins/table.go", "output file")
	flag.Parse()
	if *src == "" {
		log.Fatal("-src is required")
	}
	dir := filepath.Join(*src, "_gtfobins")
	ents, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	files := map[string]file{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			log.Fatal(err)
		}
		var f file
		// Each file is a YAML front matter document between "---" and "...".
		if err := yaml.NewDecoder(bytes.NewReader(b)).Decode(&f); err != nil {
			log.Fatalf("%s: %v", e.Name(), err)
		}
		files[e.Name()] = f
	}

	memo := map[string]result{}
	var resolve func(name string, seen map[string]bool) result
	resolve = func(name string, seen map[string]bool) result {
		if r, ok := memo[name]; ok {
			return r
		}
		if seen[name] {
			return result{}
		}
		seen[name] = true
		f, ok := files[name]
		if !ok {
			return result{}
		}
		if f.Alias != "" {
			return resolve(f.Alias, seen)
		}
		var r result
		for fn, es := range f.Functions {
			for _, e := range es {
				if _, ok := e.Contexts["sudo"]; !ok {
					continue
				}
				switch {
				case fn == "inherit" && pagers[e.From]:
					r.funcs = append(r.funcs, "Pager")
				case fn == "inherit":
					in := resolve(e.From, maps.Clone(seen))
					r.funcs = append(r.funcs, in.funcs...)
					// The inheriting program reaches the other one through its own recipe (vim -c
					// ':py ...'), or by being it (vim is vi).
					inter, trig := triggers(e.Code, name)
					r.interactive = r.interactive || inter.interactive && in.interactive
					r.args = r.args || inter.interactive && in.args
					r.triggers = append(r.triggers, trig...)
					if inter.interactive && !in.interactive {
						r.triggers = append(r.triggers, in.triggers...)
					}
				default:
					if c, ok := funcs[fn]; ok {
						r.funcs = append(r.funcs, c)
					}
					if runsCode[fn] {
						inter, trig := triggers(e.Code, name)
						r.interactive = r.interactive || inter.interactive
						r.args = r.args || inter.args
						r.triggers = append(r.triggers, trig...)
					}
				}
			}
		}
		r.interactive = r.interactive || interactive[name] && len(r.funcs) > 0
		r.args = r.args && !notArgs[name] && !r.interactive
		slices.Sort(r.funcs)
		r.funcs = slices.Compact(r.funcs)
		slices.Sort(r.triggers)
		r.triggers = slices.Compact(r.triggers)
		r.triggers = slices.DeleteFunc(r.triggers, func(t string) bool { return slices.Contains(noise[name], t) })
		memo[name] = r
		return r
	}

	names := slices.Sorted(maps.Keys(files))
	rev := "unknown"
	if b, err := exec.Command("git", "-C", *src, "rev-parse", "HEAD").Output(); err == nil {
		rev = strings.TrimSpace(string(b))
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by tools/gengtfobins from GTFOBins %s. DO NOT EDIT.\n", rev)
	fmt.Fprintf(&b, "// GTFOBins: https://gtfobins.github.io (GPL-3.0).\n\npackage gtfobins\n\nvar table = map[string]Entry{\n")
	for _, n := range names {
		r := resolve(n, map[string]bool{})
		if len(r.funcs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\t%q: {Funcs: %s", n, strings.Join(r.funcs, " | "))
		if r.interactive {
			b.WriteString(", Interactive: true")
		}
		if r.args {
			b.WriteString(", Args: true")
		}
		if len(r.triggers) > 0 {
			fmt.Fprintf(&b, ", Triggers: %#v", r.triggers)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	out2, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, out2, 0o644); err != nil {
		log.Fatal(err)
	}
}

type kind struct{ interactive, args bool }

// triggers finds the calls to name in a recipe. A call with no options or subcommand and no
// payload in its arguments reaches the shell interactively, as in "less /etc/hosts" then
// "!/bin/sh". One with the payload as a plain operand runs code from its arguments
// (awk 'BEGIN {system(...)}'). Otherwise the call's options (cut at "=") and a leading subcommand
// word are the triggers.
func triggers(code, name string) (k kind, trig []string) {
	f, err := syntax.NewParser().Parse(strings.NewReader(code), "")
	if err != nil {
		return k, nil
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		c, ok := n.(*syntax.CallExpr)
		if !ok || len(c.Args) == 0 || filepath.Base(c.Args[0].Lit()) != name {
			return true
		}
		var opts []string
		sub := ""
		carries := false
		for i, w := range c.Args[1:] {
			src := source(w)
			for _, p := range payload {
				carries = carries || strings.Contains(src, p)
			}
			a := litPrefix(w)
			switch {
			case strings.HasPrefix(a, "-") && a != "-":
				k, _, _ := strings.Cut(a, "=")
				if !generic[k] {
					opts = append(opts, k)
				}
			case i == 0 && a != "" && a == src && !strings.ContainsAny(a, "/.:") && strings.Trim(a, "0123456789") != "" && a != "x" && a != "localhost":
				sub = a
			}
		}
		// The env vars a recipe sets (PAGER=..., LESSOPEN=...) are handled by the env-hijack rule.
		if len(opts) == 0 && sub == "" && len(c.Assigns) == 0 {
			if carries {
				k.args = true
			} else {
				k.interactive = true
			}
		}
		trig = append(trig, opts...)
		if sub != "" {
			trig = append(trig, sub)
		}
		return true
	})
	return k, trig
}

// litPrefix returns the leading literal text of a word: "--eval=" for --eval='...'.
func litPrefix(w *syntax.Word) string {
	if l, ok := w.Parts[0].(*syntax.Lit); ok {
		return l.Value
	}
	return ""
}

func source(w *syntax.Word) string {
	var b strings.Builder
	syntax.NewPrinter().Print(&b, w)
	return b.String()
}
