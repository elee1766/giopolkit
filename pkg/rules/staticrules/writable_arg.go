package staticrules

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/sys/fsperm"
)

// WritableArg flags arguments naming files the requesting user could change after approval.
type WritableArg struct{}

func (WritableArg) Name() string { return "writable-arg" }

func (rule WritableArg) Check(r *rules.Request) []rules.Finding {
	var out []rules.Finding
	for i := 1; i < len(r.Argv); i++ {
		for _, c := range rules.PathCandidates(r.Argv[i], r.Cwd) {
			if mod, via := fsperm.CanModify(c, r.UID, r.Groups); mod {
				out = append(out, rules.Flag(rule, i, "argument %d (%s) can be modified by the requesting user (via %s)", i, c, via))
				break
			}
		}
	}
	return out
}
