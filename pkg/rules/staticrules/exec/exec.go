// Package exec has rules about commands that run other code: shells, interpreters, scripts, and
// programs with shell escapes.
package exec

import "github.com/elee1766/giopolkit/pkg/rules"

// Rules returns the pack's rules.
func Rules() []rules.Rule {
	return []rules.Rule{ScriptFile{}, ScriptProgram{}, PipeToShell{}, DynamicScript{}, EnvHijack{}, ShellEscape{}}
}
