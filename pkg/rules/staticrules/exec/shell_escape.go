package exec

import (
	"slices"
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
	"github.com/elee1766/giopolkit/pkg/rules/gtfobins"
)

// ShellEscape flags programs that can run arbitrary commands as root, using GTFOBins data:
//
//   - shells, interpreters, and programs whose operands are code (awk) run what the arguments say;
//   - programs with an interactive escape (vi, less, ftp) give a root shell to whoever uses them;
//   - other programs are flagged only when the arguments use an option GTFOBins uses to run code
//     (tar --checkpoint-action, find -exec, git -p);
//   - programs that open a pager (systemctl, journalctl) are flagged unless --no-pager is given.
//
// Wrappers (env, nice, sudo) are not flagged themselves: the command they run is.
type ShellEscape struct{}

func (ShellEscape) Name() string { return "shell-escape" }

func (rule ShellEscape) Check(r *rules.Request) []rules.Finding {
	cmds := r.Commands()
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		base := c.Base()
		if wrapsAnother(c, cmds) {
			return nil
		}
		flag := func(arg int, format string, a ...any) []rules.Finding {
			return []rules.Finding{rules.Flag(rule, c.Arg(arg), format, a...)}
		}
		if rules.IsInterpreter(base) {
			switch {
			case rules.ScriptFile(c.Argv) > 0 || c.PipedFrom != nil:
				return nil // script-file and pipe-to-shell say more
			case isShellScript(c):
				return nil // the script's commands are checked on their own
			case !hasOperand(c.Argv):
				return flag(0, "%s starts an interactive root shell%s", base, rules.Where(c))
			}
			return flag(0, "%s runs code from its arguments as root%s: read them carefully", base, rules.Where(c))
		}
		e, ok := gtfobins.Lookup(rules.CanonicalName(base))
		if !ok {
			return nil
		}
		switch {
		case e.Funcs&gtfobins.RunsCode == 0:
		case e.Interactive:
			return flag(0, "%s can start a root shell from its interface%s (GTFOBins)", base, rules.Where(c))
		case e.Args && hasOperand(c.Argv):
			return flag(0, "%s runs its arguments as code%s: read them carefully", base, rules.Where(c))
		default:
			if i := e.Trigger(c.Argv); i > 0 {
				return flag(i, "%s %s can run any command as root%s (GTFOBins)", base, c.Argv[i], rules.Where(c))
			}
		}
		if e.Funcs&gtfobins.Pager != 0 && pages(c) && !noPager(c) {
			return flag(0, "%s may open a pager%s, which can start a root shell (!sh in less): add --no-pager", base, rules.Where(c))
		}
		return nil
	})
}

// pagingSubcommands are the subcommands that send output through a pager. Programs not listed
// page for every invocation (journalctl, dmesg -H).
var pagingSubcommands = map[string][]string{
	"systemctl":   {"status", "show", "cat", "list-units", "list-unit-files", "list-dependencies", "list-timers", "list-sockets", "list-jobs", "help", ""},
	"git":         {"log", "diff", "show", "blame", "help", "branch", "tag", "grep", "shortlog", "reflog", "config"},
	"apt-get":     {"changelog"},
	"apt":         {"changelog", "show", "list", "search"},
	"dpkg":        {"-l", "--list"},
	"loginctl":    {"", "list-sessions", "list-users", "session-status", "user-status", "show-session", "show-user"},
	"busctl":      {"", "list", "tree", "introspect", "status"},
	"resolvectl":  {"", "status", "statistics"},
	"bootctl":     {"", "status", "list"},
	"coredumpctl": {"", "list", "info", "debug"},
	"machinectl":  {"", "list", "status", "show"},
	"networkctl":  {"", "list", "status"},
	"timedatectl": {"", "status", "show"},
	"hostnamectl": {"", "status"},
	"localectl":   {"", "status", "list-keymaps", "list-locales"},
}

// pages reports whether the command will pipe its output through a pager.
func pages(c *cmdline.Command) bool {
	subs, ok := pagingSubcommands[c.Base()]
	if !ok {
		return true
	}
	sub := ""
	for _, a := range c.Argv[1:] {
		// dpkg takes its action as an option (-l). Everything else uses a subcommand word.
		if !strings.HasPrefix(a, "-") || c.Base() == "dpkg" {
			sub = a
			break
		}
	}
	return slices.Contains(subs, sub)
}

func isShellScript(c *cmdline.Command) bool {
	_, _, ok := cmdline.ShellScript(c.Argv)
	return ok
}

// noPager reports whether the command turns its pager off.
func noPager(c *cmdline.Command) bool {
	for _, a := range c.Argv[1:] {
		if a == "--no-pager" || a == "-P" && c.Base() == "git" {
			return true
		}
	}
	for _, kv := range c.Env {
		if kv == "PAGER=cat" || kv == "SYSTEMD_PAGER=" || kv == "SYSTEMD_PAGER=cat" {
			return true
		}
	}
	return false
}

// wrapsAnother reports whether c is a wrapper (env, nice, sudo) whose wrapped command is also in
// cmds, so only the inner command needs a finding.
func wrapsAnother(c *cmdline.Command, cmds []cmdline.Command) bool {
	for i := range cmds {
		n := &cmds[i]
		if len(n.Via) == len(c.Via)+1 && len(n.Src) > 0 && len(c.Src) > 0 && n.Script == c.Script &&
			n.Src[0] > c.Src[0] && n.Src[len(n.Src)-1] == c.Src[len(c.Src)-1] {
			return true
		}
	}
	return false
}

// hasOperand reports whether argv has any argument after the program, other than -i/-l style flags.
func hasOperand(argv []string) bool {
	for _, a := range argv[1:] {
		switch a {
		case "-i", "-l", "--login", "-", "--norc", "--noprofile":
		default:
			return true
		}
	}
	return false
}
