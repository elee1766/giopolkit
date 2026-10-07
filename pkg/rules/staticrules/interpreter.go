package staticrules

import (
	"github.com/elee1766/giopolkit/pkg/rules"
)

// Interpreter flags shells and interpreters. When one runs a file, that file is highlighted since
// its contents aren't shown. Otherwise the code is in the arguments and needs a careful read.
type Interpreter struct{}

func (Interpreter) Name() string { return "interpreter" }

func (rule Interpreter) Check(r *rules.Request) []rules.Finding {
	if !rules.IsInterpreter(r.Base()) {
		return nil
	}
	if i := rules.ScriptFile(r.Argv); i > 0 {
		return []rules.Finding{rules.Flag(rule, i, "%s runs the file %s: its contents are not shown, so you can't see what will run", r.Base(), r.Argv[i])}
	}
	return []rules.Finding{rules.Flag(rule, -1, "%s runs code from its arguments: read them carefully", r.Base())}
}
