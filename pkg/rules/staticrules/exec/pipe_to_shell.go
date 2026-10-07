package exec

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// PipeToShell flags a shell or interpreter reading code from a pipe, as in curl ... | sh.
type PipeToShell struct{}

func (PipeToShell) Name() string { return "pipe-to-shell" }

func (rule PipeToShell) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		if c.PipedFrom == nil || !rules.IsInterpreter(c.Base()) || rules.ScriptFile(c.Argv) > 0 {
			return nil
		}
		return []rules.Finding{rules.Flag(rule, c.Arg(0), "%s runs code piped from %s: you can't see what will run", c.Base(), c.PipedFrom.Base())}
	})
}
