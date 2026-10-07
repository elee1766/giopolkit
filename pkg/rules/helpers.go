package rules

import (
	"os"
	"path/filepath"
	"strings"
)

// PathCandidates returns the absolute paths an argument may refer to: itself, or the value after
// "=" in --opt=value or of=value. Relative names are resolved against cwd when they exist.
func PathCandidates(a, cwd string) []string {
	vals := []string{a}
	if k := strings.IndexByte(a, '='); k >= 0 {
		vals = append(vals, a[k+1:])
	}
	var out []string
	for _, v := range vals {
		switch {
		case filepath.IsAbs(v):
			out = append(out, v)
		case v != "" && filepath.IsAbs(cwd) && !strings.HasPrefix(v, "-"):
			p := filepath.Join(cwd, v)
			if _, err := os.Lstat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// ScriptFile returns the index of the file an interpreter will run, or 0 when the code is inline
// (-c, -e) or there is no file argument. Interpreter options before the file are skipped.
func ScriptFile(argv []string) int {
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		switch a {
		case "-c", "-e", "-E", "--command", "--eval", "-C":
			return 0
		}
		if strings.HasPrefix(a, "-") {
			if !strings.HasPrefix(a, "--") && strings.ContainsAny(a[1:], "ce") {
				return 0 // combined short flags like -ec or -xc
			}
			continue
		}
		return i
	}
	return 0
}

// IsScript reports whether path starts with "#!".
func IsScript(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 2)
	n, _ := f.Read(head)
	return n == 2 && string(head) == "#!"
}

// IsInterpreter reports whether a program runs code given as a file or an argument: shells and
// language interpreters. Programs that run commands in other ways are covered by GTFOBins data.
func IsInterpreter(base string) bool {
	switch CanonicalName(base) {
	case "sh", "bash", "dash", "zsh", "ksh", "mksh", "ash", "fish", "csh", "tcsh", "busybox",
		"perl", "python", "ruby", "node", "deno", "bun", "lua", "php", "tclsh", "wish", "Rscript", "julia", "osascript":
		return true
	}
	return false
}

// CanonicalName strips version suffixes so python3.12, perl5.38, and lua5.4 match their
// unversioned names.
func CanonicalName(base string) string {
	for _, p := range []string{"python", "perl", "ruby", "lua", "php", "pip", "node", "tclsh"} {
		if rest, ok := strings.CutPrefix(base, p); ok && strings.Trim(rest, "0123456789.") == "" {
			return p
		}
	}
	return base
}
