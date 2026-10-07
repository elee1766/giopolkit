package staticrules

import (
	"github.com/elee1766/giopolkit/pkg/rules"
)

// ScriptProgram flags a program that is itself a script, whose contents the window doesn't show.
type ScriptProgram struct{}

func (ScriptProgram) Name() string { return "script-program" }

func (rule ScriptProgram) Check(r *rules.Request) []rules.Finding {
	if rules.IsInterpreter(r.Base()) || !rules.IsScript(r.Program()) {
		return nil
	}
	return []rules.Finding{rules.Flag(rule, 0, "%s is a script: its contents are not shown, so you can't see what will run", r.Program())}
}
