package exec

import (
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// EnvHijack flags environment variables that make programs load code or run other commands:
// LD_PRELOAD, PAGER, EDITOR, PYTHONPATH, and similar. pkexec clears the environment, so these
// only appear when set inside the command (env VAR=... cmd, VAR=... cmd in sh -c).
type EnvHijack struct{}

func (EnvHijack) Name() string { return "env-hijack" }

var hijackVars = map[string]string{
	"LD_PRELOAD": "loads a library into", "LD_LIBRARY_PATH": "changes where libraries are loaded for",
	"LD_AUDIT": "loads a library into", "PAGER": "sets the pager command for", "MANPAGER": "sets the pager command for",
	"GIT_PAGER": "sets the pager command for", "SYSTEMD_PAGER": "sets the pager command for", "LESSOPEN": "runs a command from",
	"EDITOR": "sets the editor command for", "VISUAL": "sets the editor command for", "SYSTEMD_EDITOR": "sets the editor command for",
	"GIT_SSH_COMMAND": "sets the ssh command for", "GIT_EXEC_PATH": "changes where git finds programs for",
	"PYTHONPATH": "changes where modules load for", "PYTHONSTARTUP": "runs a file in", "PERL5LIB": "changes where modules load for",
	"PERL5OPT": "adds options to", "RUBYLIB": "changes where libraries load for", "RUBYOPT": "adds options to",
	"NODE_OPTIONS": "adds options to", "BASH_ENV": "runs a file in", "ENV": "runs a file in", "PATH": "changes program lookup for",
	"IFS": "changes word splitting in", "PS4": "can run commands in", "PROMPT_COMMAND": "runs a command in",
}

func (rule EnvHijack) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		var out []rules.Finding
		for _, kv := range c.Env {
			k, v, _ := strings.Cut(kv, "=")
			if what, ok := hijackVars[k]; ok {
				out = append(out, rules.Flag(rule, c.Arg(0), "%s=%s %s %s%s", k, v, what, c.Base(), rules.Where(c)))
			}
		}
		return out
	})
}
