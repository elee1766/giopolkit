// Package cmdline finds the commands a command line will actually run: through wrappers such as
// env, nice, timeout, and systemd-run, and inside shell scripts passed with sh -c.
package cmdline

import (
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Command is one command that will run.
type Command struct {
	Argv []string
	// Src maps each Argv element to its index in the original argv, or -1 when it came from
	// inside a shell script argument (then Script is that index).
	Src    []int
	Script int      // index of the sh -c argument this command was parsed from, or -1
	Via    []string // wrappers it was reached through, outermost first, e.g. ["env", "sh -c"]
	// Writes lists redirect targets (> file, >> file) of a shell command.
	Writes []string
	// PipedFrom is set when stdin comes from another command in a pipeline, e.g. curl ... | sh.
	PipedFrom *Command
	// Env holds NAME=VALUE assignments that apply to this command (env A=b cmd, A=b cmd in a script).
	Env []string
	// Dynamic is set on a sh -c command whose script failed to parse or uses expansions
	// ($VAR, $(cmd), `cmd`), so what it runs can't be read from the text alone.
	Dynamic bool
}

// Base returns the base name of the program.
func (c *Command) Base() string {
	if len(c.Argv) == 0 {
		return ""
	}
	return filepath.Base(c.Argv[0])
}

// Arg returns the original argv index for c.Argv[i]: the argument itself, or the shell script
// argument it came from.
func (c *Command) Arg(i int) int {
	if i >= 0 && i < len(c.Src) && c.Src[i] >= 0 {
		return c.Src[i]
	}
	return c.Script
}

// Expand returns the outer command and every command reached through wrappers and shell scripts.
// The outer command is first.
func Expand(argv []string) []Command {
	src := make([]int, len(argv))
	for i := range src {
		src[i] = i
	}
	var out []Command
	expand(Command{Argv: argv, Src: src, Script: -1}, &out, 0)
	return out
}

const maxDepth = 8

func expand(c Command, out *[]Command, depth int) {
	if len(c.Argv) == 0 {
		return
	}
	if depth >= maxDepth {
		*out = append(*out, c)
		return
	}
	base := c.Base()
	if script, i, ok := ShellScript(c.Argv); ok {
		cmds, dynamic := parseScript(script)
		c.Dynamic = dynamic
		*out = append(*out, c)
		via := append(append([]string(nil), c.Via...), base+" -c")
		scriptIdx := c.Arg(i)
		for _, sc := range cmds {
			sc.Script, sc.Via = scriptIdx, via
			expand(sc, out, depth+1)
		}
		return
	}
	*out = append(*out, c)
	if start := unwrap(base, c.Argv); start > 0 && start < len(c.Argv) {
		env := append([]string(nil), c.Env...)
		if wrappers[base].assigns {
			for _, a := range c.Argv[1:start] {
				if isAssign(a) {
					env = append(env, a)
				}
			}
		}
		inner := Command{
			Argv:   c.Argv[start:],
			Src:    c.Src[start:],
			Script: c.Script,
			Via:    append(append([]string(nil), c.Via...), base),
			Env:    env,
		}
		expand(inner, out, depth+1)
	}
}

var shells = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "mksh": true, "ash": true, "busybox": true}

// ShellScript returns the script of sh -c SCRIPT and its argv index.
func ShellScript(argv []string) (string, int, bool) {
	base := filepath.Base(argv[0])
	args := argv[1:]
	off := 1
	if base == "busybox" {
		if len(args) == 0 || !shells[args[0]] {
			return "", 0, false
		}
		args, off = args[1:], 2
	} else if !shells[base] {
		return "", 0, false
	}
	for i, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") {
			return "", 0, false
		}
		if a == "-c" || (!strings.HasPrefix(a, "--") && strings.Contains(a[1:], "c")) {
			if i+1 < len(args) {
				return args[i+1], off + i + 1, true
			}
			return "", 0, false
		}
	}
	return "", 0, false
}

// wrapper describes a command that runs another command given as its trailing arguments.
type wrapper struct {
	valueOpts  string   // short options that take a value
	longVal    []string // long options that take a separate value
	positional int      // positional arguments before the command (timeout DURATION)
	assigns    bool     // NAME=VALUE arguments before the command (env)
}

var wrappers = map[string]wrapper{
	"env":         {valueOpts: "uCSP", longVal: []string{"--unset", "--chdir", "--split-string"}, assigns: true},
	"nice":        {valueOpts: "n", longVal: []string{"--adjustment"}},
	"ionice":      {valueOpts: "cnp", longVal: []string{"--class", "--classdata", "--pid"}},
	"nohup":       {},
	"setsid":      {},
	"stdbuf":      {valueOpts: "ioe", longVal: []string{"--input", "--output", "--error"}},
	"timeout":     {valueOpts: "sk", longVal: []string{"--signal", "--kill-after"}, positional: 1},
	"time":        {valueOpts: "fo", longVal: []string{"--format", "--output"}},
	"chrt":        {positional: 1},
	"taskset":     {positional: 1},
	"numactl":     {},
	"unshare":     {},
	"chroot":      {positional: 1},
	"flock":       {valueOpts: "wE", positional: 1},
	"xargs":       {valueOpts: "adeEiIlLnPs", longVal: []string{"--arg-file", "--delimiter", "--max-args", "--max-procs", "--max-chars"}},
	"systemd-run": {valueOpts: "puMEH", longVal: []string{"--unit", "--property", "--description", "--slice", "--uid", "--gid", "--nice", "--working-directory", "--setenv", "--machine", "--host", "--on-calendar", "--on-active", "--timer-property", "--path-property", "--socket-property", "--service-type"}},
	"sudo":        {valueOpts: "ugCDhpRTUrt", longVal: []string{"--user", "--group", "--chdir", "--host", "--prompt", "--chroot", "--other-user", "--close-from", "--role", "--type", "--command-timeout"}, assigns: true},
	"doas":        {valueOpts: "uC"},
	"pkexec":      {valueOpts: "u", longVal: []string{"--user"}},
	"run0":        {valueOpts: "uDgM", longVal: []string{"--user", "--group", "--chdir", "--machine", "--property", "--unit", "--description", "--slice", "--nice", "--setenv", "--background"}},
	"watch":       {valueOpts: "dnq", longVal: []string{"--interval", "--differences", "--equexit"}},
	"strace":      {valueOpts: "eoOpPsSuEIabX", longVal: []string{"--output", "--attach", "--user"}},
	"ltrace":      {valueOpts: "eoOpPsSuEIabX", longVal: []string{"--output"}},
	"firejail":    {},
	"bwrap":       {},
	"capsh":       {},
}

// unwrap returns the index where the wrapped command starts, or 0 if base is not a wrapper or no
// command follows. Unknown options are assumed not to take a value.
func unwrap(base string, argv []string) int {
	w, ok := wrappers[base]
	if !ok {
		return 0
	}
	pos := w.positional
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			if i+1 < len(argv) && pos == 0 {
				return i + 1
			}
		case strings.HasPrefix(a, "--"):
			name, _, hasVal := strings.Cut(a, "=")
			if !hasVal && slices.Contains(w.longVal, name) {
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// -n 5, -n5, -xn 5: the first value-taking letter consumes the rest or the next arg.
			for j := 1; j < len(a); j++ {
				if strings.IndexByte(w.valueOpts, a[j]) >= 0 {
					if j == len(a)-1 {
						i++
					}
					break
				}
			}
		case w.assigns && isAssign(a):
		case pos > 0:
			pos--
		default:
			return i
		}
	}
	return 0
}

func isAssign(a string) bool {
	k, _, ok := strings.Cut(a, "=")
	if !ok || k == "" {
		return false
	}
	for i, r := range k {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// parseScript returns the simple commands in a shell script. Words that aren't plain literals
// (variables, command substitutions) are kept as their source text. A script that fails to parse
// gives no commands. dynamic reports a parse failure or any expansion in the script.
func parseScript(script string) (cmds []Command, dynamic bool) {
	f, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return nil, true
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp:
			dynamic = true
		}
		return !dynamic
	})
	var out []Command
	var stmts func(s *syntax.Stmt, pipedFrom *Command)
	stmts = func(s *syntax.Stmt, pipedFrom *Command) {
		if s == nil {
			return
		}
		var writes []string
		for _, r := range s.Redirs {
			switch r.Op {
			case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.RdrAll, syntax.AppAll:
				if r.Word != nil {
					writes = append(writes, wordString(r.Word))
				}
			}
		}
		if s.Cmd == nil {
			// A bare redirect such as "> /var/log/x" truncates the file. It is recorded as a
			// command with only Writes.
			if len(writes) > 0 {
				out = append(out, Command{Argv: []string{":"}, Src: []int{-1}, Writes: writes})
			}
			return
		}
		switch c := s.Cmd.(type) {
		case *syntax.BinaryCmd:
			if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
				before := len(out)
				stmts(c.X, pipedFrom)
				var left *Command
				if len(out) > before {
					l := out[len(out)-1]
					left = &l
				}
				stmts(c.Y, left)
				return
			}
			stmts(c.X, nil)
			stmts(c.Y, nil)
			return
		case *syntax.CallExpr:
			cmd := Command{PipedFrom: pipedFrom, Writes: writes}
			for _, a := range c.Assigns {
				if a.Name != nil && a.Value != nil {
					cmd.Env = append(cmd.Env, a.Name.Value+"="+wordString(a.Value))
				}
			}
			for _, w := range c.Args {
				cmd.Argv = append(cmd.Argv, wordString(w))
				cmd.Src = append(cmd.Src, -1)
			}
			if len(cmd.Argv) > 0 {
				out = append(out, cmd)
			}
		}
		// Nested statements: subshells, blocks, if/for/while bodies, command substitutions.
		syntax.Walk(s.Cmd, func(n syntax.Node) bool {
			switch n := n.(type) {
			case *syntax.Stmt:
				if n != s {
					stmts(n, nil)
					return false
				}
			case *syntax.CallExpr:
				return n == s.Cmd
			}
			return true
		})
	}
	for _, s := range f.Stmts {
		stmts(s, nil)
	}
	return out, dynamic
}

// wordString returns a word's value: literal and quoted parts are unquoted, anything dynamic is
// kept as source text.
func wordString(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				if l, ok := q.(*syntax.Lit); ok {
					b.WriteString(l.Value)
				} else {
					b.WriteString(nodeString(q))
				}
			}
		default:
			b.WriteString(nodeString(p))
		}
	}
	return b.String()
}

func nodeString(n syntax.Node) string {
	var b strings.Builder
	syntax.NewPrinter().Print(&b, n)
	return b.String()
}
