package exec

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// ScriptFile flags an interpreter running a file, whose contents the window doesn't show.
type ScriptFile struct{}

func (ScriptFile) Name() string { return "script-file" }

func (rule ScriptFile) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		if !rules.IsInterpreter(c.Base()) {
			return nil
		}
		i := rules.ScriptFile(c.Argv)
		if i == 0 {
			return nil
		}
		return []rules.Finding{rules.Flag(rule, c.Arg(i), "%s runs the file %s%s: its contents are not shown, so you can't see what will run", c.Base(), c.Argv[i], rules.Where(c))}
	})
}
