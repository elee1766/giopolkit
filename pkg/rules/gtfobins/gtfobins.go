// Package gtfobins says what an executable can do when run as root, from GTFOBins data.
package gtfobins

//go:generate sh -c "test -n \"$GTFOBINS\" || { echo set GTFOBINS to a GTFOBins checkout; exit 1; }; go run ../../../tools/gengtfobins -src \"$GTFOBINS\" -out table.go"

import "strings"

// Func is a set of GTFOBins functions.
type Func uint16

const (
	Shell Func = 1 << iota
	Command
	ReverseShell
	BindShell
	FileWrite
	FileRead
	LibraryLoad
	Upload
	Download
	PrivilegeEscalation
	Pager // opens a pager such as less, which can start a shell from an interactive session
)

// RunsCode is the set of functions that let the caller run arbitrary programs.
const RunsCode = Shell | Command | ReverseShell | BindShell | LibraryLoad

// Entry is what GTFOBins knows about one executable.
type Entry struct {
	Funcs Func
	// Interactive is set when the program can start a shell from its own interface with no
	// special arguments (vi, less, ftp).
	Interactive bool
	// Args is set when the program runs code given as a plain operand (awk, env).
	Args bool
	// Triggers are the options and subcommands GTFOBins recipes use to run code, such as
	// "--checkpoint-action" for tar or "-exec" for find.
	Triggers []string
}

var names = []struct {
	f    Func
	name string
}{
	{Shell, "shell"}, {Command, "command"}, {ReverseShell, "reverse-shell"}, {BindShell, "bind-shell"},
	{LibraryLoad, "library-load"}, {FileWrite, "file-write"}, {FileRead, "file-read"},
	{Upload, "upload"}, {Download, "download"}, {PrivilegeEscalation, "privilege-escalation"},
	{Pager, "pager"},
}

// Lookup returns what GTFOBins knows about the named executable (a base name).
func Lookup(name string) (Entry, bool) {
	e, ok := table[name]
	return e, ok
}

// Trigger returns the index of the first argument that matches a trigger, or -1. Options match by
// name ("--opt" and "--opt=value"), short options also when the value is attached ("-Icmd").
func (e Entry) Trigger(argv []string) int {
	for i, a := range argv {
		if i == 0 {
			continue
		}
		for _, t := range e.Triggers {
			switch {
			case a == t:
				return i
			case strings.HasPrefix(t, "--") && strings.HasPrefix(a, t+"="):
				return i
			case len(t) == 2 && t[0] == '-' && t[1] != '-' && strings.HasPrefix(a, t):
				return i
			}
		}
	}
	return -1
}

func (f Func) String() string {
	var out []string
	for _, n := range names {
		if f&n.f != 0 {
			out = append(out, n.name)
		}
	}
	return strings.Join(out, ", ")
}
