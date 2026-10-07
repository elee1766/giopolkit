package system

import (
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// LogTamper flags deleting or emptying system logs and shell history.
type LogTamper struct{}

func (LogTamper) Name() string { return "log-tamper" }

func (rule LogTamper) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		base := c.Base()
		if base == "journalctl" {
			for i, a := range c.Argv[1:] {
				if strings.HasPrefix(a, "--vacuum") {
					return []rules.Finding{rules.Flag(rule, c.Arg(i+1), "journalctl %s deletes journal logs%s", a, rules.Where(c))}
				}
			}
			return nil
		}
		isLog := func(p string) bool {
			return strings.HasPrefix(p, "/var/log") || strings.HasSuffix(p, "_history") || strings.HasSuffix(p, "/wtmp") || strings.HasSuffix(p, "/btmp") || strings.HasSuffix(p, "/lastlog")
		}
		var out []rules.Finding
		// "> /var/log/x", ": > x", "echo > x", "cat /dev/null > x" empty the file.
		empties := len(c.Argv) == 1 || base == "echo" || base == "true" || base == ":" ||
			base == "cat" && len(c.Argv) == 2 && c.Argv[1] == "/dev/null"
		for _, w := range c.Writes {
			if empties && isLog(w) {
				out = append(out, rules.Flag(rule, c.Arg(-1), "%s empties %s%s", base, w, rules.Where(c)))
			}
		}
		switch base {
		case "rm", "shred", "truncate", "unlink":
			for i, a := range c.Argv[1:] {
				if !strings.HasPrefix(a, "-") && isLog(a) {
					out = append(out, rules.Flag(rule, c.Arg(i+1), "%s removes log %s%s", base, a, rules.Where(c)))
				}
			}
		}
		return out
	})
}
