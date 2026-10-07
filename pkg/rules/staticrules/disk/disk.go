// Package disk has rules about writing to disks and partitions.
package disk

import "github.com/elee1766/giopolkit/pkg/rules"

// Rules returns the pack's rules.
func Rules() []rules.Rule { return []rules.Rule{RawBlockWrite{}} }
