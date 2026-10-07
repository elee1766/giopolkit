// Package staticrules collects the built-in rule packs. Each pack is a subpackage with one file
// per rule.
package staticrules

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/staticrules/disk"
	"github.com/elee1766/giopolkit/pkg/rules/staticrules/exec"
	"github.com/elee1766/giopolkit/pkg/rules/staticrules/fs"
	"github.com/elee1766/giopolkit/pkg/rules/staticrules/integrity"
	"github.com/elee1766/giopolkit/pkg/rules/staticrules/system"
)

// Default returns the built-in rules in the order their findings are shown: destructive actions
// first, then code execution, then integrity of what is shown.
func Default() []rules.Rule {
	var rs []rules.Rule
	rs = append(rs, disk.Rules()...)
	rs = append(rs, fs.Rules()...)
	rs = append(rs, system.Rules()...)
	rs = append(rs, exec.Rules()...)
	rs = append(rs, integrity.Rules()...)
	return rs
}
