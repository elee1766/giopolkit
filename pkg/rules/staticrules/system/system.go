// Package system has rules about turning off security features, adding things that run at boot,
// and removing logs. Patterns are adapted from SigmaHQ Linux process_creation rules.
package system

import "github.com/elee1766/giopolkit/pkg/rules"

// Rules returns the pack's rules.
func Rules() []rules.Rule { return []rules.Rule{SecurityOff{}, LogTamper{}, KernelModule{}} }
