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

// IsInterpreter reports whether a program base name runs code taken from its arguments.
func IsInterpreter(base string) bool {
	switch base {
	case "sh", "bash", "dash", "zsh", "fish", "env", "perl", "ruby", "node", "deno", "bun", "lua", "php", "xargs", "find", "busybox", "nsenter", "systemd-run", "su", "sudo", "doas", "pkexec", "make", "awk", "gawk", "tclsh":
		return true
	}
	return strings.HasPrefix(base, "python")
}
