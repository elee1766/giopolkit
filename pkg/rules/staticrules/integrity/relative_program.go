package integrity

import (
	"path/filepath"

	"github.com/elee1766/giopolkit/pkg/rules"
)

// RelativeProgram flags a program name without a path: pkexec resolves it with the caller's PATH.
type RelativeProgram struct{}

func (RelativeProgram) Name() string { return "relative-program" }

func (rule RelativeProgram) Check(r *rules.Request) []rules.Finding {
	if p := r.Program(); p == "" || filepath.IsAbs(p) {
		return nil
	}
	return []rules.Finding{rules.Flag(rule, 0, "relative program name: pkexec looks it up in the caller's PATH, which the caller controls")}
}
