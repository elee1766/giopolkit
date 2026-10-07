// Package staticrules holds the built-in rules. Each exported rule lives in its own file.
package staticrules

import "github.com/elee1766/giopolkit/pkg/rules"

// Default returns the built-in rules in the order their findings are shown.
func Default() []rules.Rule {
	return []rules.Rule{
		RawBlockWrite{},
		RelativeProgram{},
		WritableProgram{},
		WritableArg{},
		ScriptProgram{},
		Interpreter{},
	}
}
