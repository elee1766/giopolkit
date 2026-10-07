// Package fs has rules about deleting and overwriting important files. Patterns are adapted from
// Destructive Command Guard (github.com/Dicklesworthstone/destructive_command_guard) and SigmaHQ
// Linux process_creation rules.
package fs

import "github.com/elee1766/giopolkit/pkg/rules"

// Rules returns the pack's rules.
func Rules() []rules.Rule { return []rules.Rule{RecursiveDelete{}, SensitiveWrite{}, Setuid{}} }
