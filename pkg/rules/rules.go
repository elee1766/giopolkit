// Package rules defines checks that flag risky pkexec commands and helpers for writing them.
package rules

import (
	"fmt"
	"path/filepath"
)

// Request is what a rule sees: the exact command pkexec will run and who asked for it.
type Request struct {
	Argv       []string // argv pkexec will run, Argv[0] is the program
	Cwd        string   // working directory, absolute when known
	TargetUser string   // user the command runs as
	UID        int      // uid of the requesting process
	Groups     map[uint32]bool
}

// Program returns Argv[0], or "" for an empty argv.
func (r *Request) Program() string {
	if len(r.Argv) == 0 {
		return ""
	}
	return r.Argv[0]
}

// Base returns the base name of the program.
func (r *Request) Base() string { return filepath.Base(r.Program()) }

// Finding is one flagged property of a request.
type Finding struct {
	Rule string // rule name, shown in the window header
	Text string
	Arg  int // index into Argv the finding is about, or -1
}

// Rule checks a request.
type Rule interface {
	Name() string
	Check(r *Request) []Finding
}

// Flag builds a finding for rule about argument arg (-1 for none).
func Flag(rule Rule, arg int, format string, a ...any) Finding {
	return Finding{Rule: rule.Name(), Text: fmt.Sprintf(format, a...), Arg: arg}
}

// Run applies rules in order and returns all findings.
func Run(r *Request, rs ...Rule) []Finding {
	var out []Finding
	for _, rule := range rs {
		out = append(out, rule.Check(r)...)
	}
	return out
}
