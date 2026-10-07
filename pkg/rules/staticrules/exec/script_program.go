package exec

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// ScriptProgram flags a program that is itself a script, whose contents the window doesn't show.
type ScriptProgram struct{}

func (ScriptProgram) Name() string { return "script-program" }

func (rule ScriptProgram) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		if rules.IsInterpreter(c.Base()) || !rules.IsScript(c.Argv[0]) {
			return nil
		}
		return []rules.Finding{rules.Flag(rule, c.Arg(0), "%s is a script%s: its contents are not shown, so you can't see what will run", c.Argv[0], rules.Where(c))}
	})
}
