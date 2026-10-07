// Package rules defines checks that flag risky pkexec commands and helpers for writing them.
package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// Request is what a rule sees: the exact command pkexec will run and who asked for it.
type Request struct {
	Argv       []string // argv pkexec will run, Argv[0] is the program
	Cwd        string   // working directory, absolute when known
	TargetUser string   // user the command runs as
	UID        int      // uid of the requesting process
	Groups     map[uint32]bool

	cmds []cmdline.Command
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

// Commands returns the outer command followed by every command it runs through wrappers (env,
// nice, systemd-run, ...) and sh -c scripts.
func (r *Request) Commands() []cmdline.Command {
	if r.cmds == nil {
		r.cmds = cmdline.Expand(r.Argv)
	}
	return r.cmds
}

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

// Run applies rules in order and returns all findings. Identical findings are reported once.
func Run(r *Request, rs ...Rule) []Finding {
	var out []Finding
	seen := map[Finding]bool{}
	for _, rule := range rs {
		for _, f := range rule.Check(r) {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// EachCommand runs check on every command in r and collects the findings. Use it for rules about
// what a program does, so wrapped and scripted commands are checked too.
func EachCommand(r *Request, check func(c *cmdline.Command) []Finding) []Finding {
	var out []Finding
	cmds := r.Commands()
	for i := range cmds {
		out = append(out, check(&cmds[i])...)
	}
	return out
}

// Where describes how a command is reached, for finding text: "" for the outer command, or
// " (via env, sh -c)".
func Where(c *cmdline.Command) string {
	if len(c.Via) == 0 {
		return ""
	}
	return " (via " + strings.Join(c.Via, ", ") + ")"
}
