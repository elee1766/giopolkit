// Package pkexec turns a polkit request into facts worth showing: who asked, what will run, and
// whether the requester could change it after approval.
package pkexec

import (
	"strconv"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/sys/proc"
)

const ExecAction = "org.freedesktop.policykit.exec"

type Report struct {
	Program    string          // program pkexec will run, if this is a pkexec request
	Argv       []string        // exact argv pkexec will run
	TargetUser string          // pkexec --user, default root
	Cwd        string          // working directory of the command, best effort
	Trusted    bool            // Argv was read from a setuid pkexec process, which the caller can't alter
	Chain      []proc.Proc     // the requester and its parents, pkexec excluded
	Warnings   []rules.Finding // things the approver should know
}

// Build inspects a request. polkitd forwards only polkit.subject-pid and polkit.caller-pid to
// agents, not pkexec's program and command_line details. pkexec makes its parent the subject and
// calls polkitd itself, so caller-pid is the pkexec process. Its exact argv is read from
// /proc/PID/cmdline. pkexec runs setuid root, so a same-user caller can't ptrace it or rewrite its
// memory: that argv is what pkexec will run.
func Build(actionID string, details map[string]string, rs []rules.Rule) Report {
	r := Report{}
	subject, _ := strconv.Atoi(details["polkit.subject-pid"])
	caller, _ := strconv.Atoi(details["polkit.caller-pid"])
	if subject <= 0 && caller <= 0 {
		return r
	}
	isPkexec := actionID == ExecAction && caller > 0 && readPkexec(&r, caller)
	start := subject
	if start <= 0 {
		start = caller
	}
	r.Chain = proc.Chain(start, 6)
	if len(r.Chain) == 0 {
		return r
	}
	req := r.Chain[0]
	if !isPkexec {
		return r
	}
	if r.Cwd == "" {
		if cwd, err := proc.Cwd(req.PID); err == nil {
			r.Cwd = cwd
		}
	}
	if r.Program == "" {
		return r
	}
	rq := &rules.Request{Argv: r.Argv, Cwd: r.Cwd, TargetUser: r.TargetUser, UID: req.UID, Groups: proc.Groups(req.PID)}
	r.Warnings = append(r.Warnings, rules.Run(rq, rs...)...)
	return r
}

// readPkexec fills Argv, Program, TargetUser, and Cwd from a pkexec process. It returns false if
// pid is not a setuid pkexec, since only then can the argv be trusted.
func readPkexec(r *Report, pid int) bool {
	if proc.Comm(pid) != "pkexec" || proc.EUID(pid) != 0 {
		r.Warnings = append(r.Warnings, rules.Finding{Rule: "unverified-caller", Text: "the requesting process is not a setuid pkexec: the command can't be verified", Arg: -1})
		return false
	}
	args, err := proc.Cmdline(pid)
	if err != nil || len(args) == 0 {
		return false
	}
	user, keepCwd := "root", false
	i := 1
loop:
	for ; i < len(args); i++ {
		switch args[i] {
		case "--user", "-u":
			i++
			if i < len(args) {
				user = args[i]
			}
		case "--disable-internal-agent":
		case "--keep-cwd":
			keepCwd = true
		default:
			break loop
		}
	}
	r.TargetUser = user
	r.Trusted = true
	if i >= len(args) {
		r.Argv = []string{"(login shell of " + user + ")"}
		return true
	}
	r.Argv = args[i:]
	r.Program = args[i]
	if !keepCwd {
		r.Cwd = "(home of " + user + ")"
	}
	return true
}
