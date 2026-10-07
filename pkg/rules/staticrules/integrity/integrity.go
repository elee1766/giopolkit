// Package integrity has rules about whether the command shown is the command that will run: files
// the requester can change after approval, and programs found through the requester's PATH.
package integrity

import "github.com/elee1766/giopolkit/pkg/rules"

// Rules returns the pack's rules.
func Rules() []rules.Rule { return []rules.Rule{RelativeProgram{}, WritableProgram{}, WritableArg{}} }
