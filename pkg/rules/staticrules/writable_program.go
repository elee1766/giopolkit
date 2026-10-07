package staticrules

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/sys/fsperm"
)

// WritableProgram flags a program the requesting user could replace after approval.
type WritableProgram struct{}

func (WritableProgram) Name() string { return "writable-program" }

func (rule WritableProgram) Check(r *rules.Request) []rules.Finding {
	if mod, via := fsperm.CanModify(r.Program(), r.UID, r.Groups); mod {
		return []rules.Finding{rules.Flag(rule, 0, "%s can be modified by the requesting user (via %s): it could be replaced after you approve", r.Program(), via)}
	}
	return nil
}
