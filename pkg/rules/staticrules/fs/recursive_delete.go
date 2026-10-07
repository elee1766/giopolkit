package fs

import (
	"slices"
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// RecursiveDelete flags recursive removal of system directories or home directories:
// rm -r, find -delete, and shred -r.
type RecursiveDelete struct{}

func (RecursiveDelete) Name() string { return "recursive-delete" }

func (rule RecursiveDelete) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		var targets []int
		switch c.Base() {
		case "rm":
			recursive, end := false, false
			for i, a := range c.Argv[1:] {
				i++
				switch {
				case end || !strings.HasPrefix(a, "-") || a == "-":
					targets = append(targets, i)
				case a == "--":
					end = true
				case a == "--recursive" || !strings.HasPrefix(a, "--") && strings.ContainsAny(a[1:], "rR"):
					recursive = true
				}
			}
			if !recursive {
				return nil
			}
		case "find":
			if !slices.Contains(c.Argv, "-delete") && !execsRm(c.Argv) {
				return nil
			}
			for i, a := range c.Argv[1:] {
				if strings.HasPrefix(a, "-") || a == "(" || a == "!" {
					break
				}
				targets = append(targets, i+1)
			}
		default:
			return nil
		}
		var out []rules.Finding
		for _, i := range targets {
			p := clean(c.Argv[i], r.Cwd)
			if isCritical(p) {
				out = append(out, rules.Flag(rule, c.Arg(i), "%s deletes everything under %s%s", c.Base(), c.Argv[i], rules.Where(c)))
			}
		}
		return out
	})
}

func execsRm(argv []string) bool {
	for i, a := range argv {
		if (a == "-exec" || a == "-execdir") && i+1 < len(argv) && strings.HasSuffix(argv[i+1], "rm") {
			return true
		}
	}
	return false
}
