package fs

import (
	"strconv"
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// Setuid flags making a file setuid or setgid, or giving it capabilities: anyone who can run it
// then gets those privileges.
type Setuid struct{}

func (Setuid) Name() string { return "setuid" }

func (rule Setuid) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		switch c.Base() {
		case "chmod":
			for i, a := range c.Argv[1:] {
				if setsID(a) {
					return []rules.Finding{rules.Flag(rule, c.Arg(i+1), "chmod %s makes files run with their owner's privileges%s", a, rules.Where(c))}
				}
			}
		case "install":
			for i, a := range c.Argv[1:] {
				if v, ok := strings.CutPrefix(a, "--mode="); ok && setsID(v) {
					return []rules.Finding{rules.Flag(rule, c.Arg(i+1), "install %s makes files run with their owner's privileges%s", a, rules.Where(c))}
				}
				if a == "-m" && i+2 < len(c.Argv) && setsID(c.Argv[i+2]) {
					return []rules.Finding{rules.Flag(rule, c.Arg(i+2), "install -m %s makes files run with their owner's privileges%s", c.Argv[i+2], rules.Where(c))}
				}
			}
		case "setcap":
			return []rules.Finding{rules.Flag(rule, c.Arg(0), "setcap gives a program capabilities that anyone running it gets%s", rules.Where(c))}
		}
		return nil
	})
}

// setsID reports whether a chmod mode adds setuid or setgid: u+s, g+s, +s, or a 4-digit octal
// mode with 4 or 2 in the first digit.
func setsID(m string) bool {
	for _, part := range strings.Split(m, ",") {
		if i := strings.IndexAny(part, "+="); i >= 0 && strings.Contains(part[i:], "s") {
			return true
		}
	}
	if len(m) == 4 || len(m) == 5 {
		if v, err := strconv.ParseUint(m, 8, 16); err == nil {
			return v&0o6000 != 0
		}
	}
	return false
}
