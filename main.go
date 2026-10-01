// Command rootpls lets an agent ask the user to run a command as root.
// It shows a confirmation dialog, then hands off to the OS authenticator (polkit on Linux).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	exitDenied = 126
	exitUsage  = 2
)

type request struct {
	Reason  string
	Risk    string
	Argv    []string // run directly when len > 1
	Shell   string   // run via /bin/sh -c when set
	Dir     string
	Timeout time.Duration
}

func (r request) Display() string {
	if r.Shell != "" {
		return r.Shell
	}
	return shellJoin(r.Argv)
}

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("rootpls", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: rootpls -r REASON [-l low|medium|high] [-t 120s] [--dialog-only] -- COMMAND [ARGS...]

One argument after -- runs through /bin/sh -c (pipes, redirects, &&).
Several arguments run directly with no shell.
Exit codes: 0 ok, 126 denied/timeout/auth failed, 2 usage, else the command's code.
`)
	}
	var req request
	var dialogOnly bool
	fs.StringVar(&req.Reason, "r", "", "why root is needed (required)")
	fs.StringVar(&req.Reason, "reason", "", "alias for -r")
	fs.StringVar(&req.Risk, "l", "medium", "risk: low, medium, high")
	fs.StringVar(&req.Risk, "risk", "medium", "alias for -l")
	fs.DurationVar(&req.Timeout, "t", 120*time.Second, "approval timeout")
	fs.BoolVar(&dialogOnly, "dialog-only", false, "show the dialog and exit 0 if approved, without running anything")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	args := fs.Args()
	switch {
	case len(args) == 0:
		fmt.Fprintln(os.Stderr, "rootpls: missing command")
		fs.Usage()
		return exitUsage
	case strings.TrimSpace(req.Reason) == "":
		fmt.Fprintln(os.Stderr, "rootpls: -r REASON is required")
		return exitUsage
	case req.Risk != "low" && req.Risk != "medium" && req.Risk != "high":
		fmt.Fprintln(os.Stderr, "rootpls: -l must be low, medium, or high")
		return exitUsage
	}
	if len(args) == 1 {
		req.Shell = args[0]
	} else {
		req.Argv = args
	}
	req.Dir, _ = os.Getwd()

	if !confirm(req) {
		fmt.Fprintln(os.Stderr, "rootpls: request denied or timed out. Command NOT executed.")
		return exitDenied
	}
	if dialogOnly {
		return 0
	}

	cmd, err := elevate(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rootpls:", err)
		return exitDenied
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, os.Stdout, os.Stderr
	err = cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		code := ee.ExitCode()
		if code == 126 || code == 127 {
			fmt.Fprintf(os.Stderr, "rootpls: authenticator exit %d (password prompt dismissed, auth failed, or command not found)\n", code)
		}
		if code < 0 {
			return 1
		}
		return code
	default:
		fmt.Fprintln(os.Stderr, "rootpls:", err)
		return 1
	}
}

func shellJoin(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`|&;<>()*?[]{}~#!") {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}
