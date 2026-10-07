// Package proc reads process information from /proc.
package proc

import (
	"os"
	"strconv"
	"strings"
)

// Proc is a process as shown to the approver.
type Proc struct {
	PID int
	UID int // real uid
	Exe string
	Cmd string // argv joined with spaces
}

func dir(pid int) string { return "/proc/" + strconv.Itoa(pid) }

// Read returns a process, or false if it doesn't exist.
func Read(pid int) (Proc, bool) {
	p := Proc{PID: pid, UID: -1}
	st, ok := status(pid)
	if !ok {
		return p, false
	}
	if fs := strings.Fields(st["Uid"]); len(fs) > 0 {
		p.UID, _ = strconv.Atoi(fs[0])
	}
	p.Exe, _ = os.Readlink(dir(pid) + "/exe")
	if argv, err := Cmdline(pid); err == nil {
		p.Cmd = strings.Join(argv, " ")
	}
	return p, true
}

// Chain returns pid and up to max-1 of its ancestors, stopping before init.
func Chain(pid, max int) []Proc {
	var out []Proc
	for i := 0; i < max && pid > 1; i++ {
		p, ok := Read(pid)
		if !ok {
			break
		}
		out = append(out, p)
		pid = PPID(pid)
	}
	return out
}

// Cmdline returns a process's argv.
func Cmdline(pid int) ([]string, error) {
	b, err := os.ReadFile(dir(pid) + "/cmdline")
	if err != nil || len(b) == 0 {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), nil
}

// Comm returns a process's command name.
func Comm(pid int) string {
	b, _ := os.ReadFile(dir(pid) + "/comm")
	return strings.TrimSpace(string(b))
}

// Cwd returns a process's working directory.
func Cwd(pid int) (string, error) { return os.Readlink(dir(pid) + "/cwd") }

// PPID returns the parent pid, or 0.
func PPID(pid int) int {
	b, err := os.ReadFile(dir(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(f[1])
	return ppid
}

// EUID returns the effective uid, or -1.
func EUID(pid int) int {
	st, _ := status(pid)
	if fs := strings.Fields(st["Uid"]); len(fs) > 1 {
		v, _ := strconv.Atoi(fs[1])
		return v
	}
	return -1
}

// Groups returns the primary and supplementary groups of a process.
func Groups(pid int) map[uint32]bool {
	g := map[uint32]bool{}
	st, _ := status(pid)
	ids := strings.Fields(st["Groups"])
	if fs := strings.Fields(st["Gid"]); len(fs) > 0 {
		ids = append(ids, fs[0])
	}
	for _, x := range ids {
		if v, err := strconv.ParseUint(x, 10, 32); err == nil {
			g[uint32(v)] = true
		}
	}
	return g
}

func status(pid int) (map[string]string, bool) {
	b, err := os.ReadFile(dir(pid) + "/status")
	if err != nil {
		return nil, false
	}
	m := map[string]string{}
	for l := range strings.Lines(string(b)) {
		if k, v, ok := strings.Cut(l, ":"); ok {
			m[k] = strings.TrimSpace(v)
		}
	}
	return m, true
}
