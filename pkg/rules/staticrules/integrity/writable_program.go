package integrity

import (
	"path/filepath"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
	"github.com/elee1766/giopolkit/pkg/sys/fsperm"
)

// WritableProgram flags a program the requesting user could replace after approval.
type WritableProgram struct{}

func (WritableProgram) Name() string { return "writable-program" }

func (rule WritableProgram) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		p := c.Argv[0]
		if !filepath.IsAbs(p) {
			return nil
		}
		if mod, via := fsperm.CanModify(p, r.UID, r.Groups); mod {
			return []rules.Finding{rules.Flag(rule, c.Arg(0), "%s can be modified by the requesting user (via %s)%s: it could be replaced after you approve", p, via, rules.Where(c))}
		}
		return nil
	})
}
