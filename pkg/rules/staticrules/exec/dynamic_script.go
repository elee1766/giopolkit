package exec

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// DynamicScript flags sh -c scripts that use variables or command substitution, or don't parse,
// so the commands shown are not exactly what will run.
type DynamicScript struct{}

func (DynamicScript) Name() string { return "dynamic-script" }

func (rule DynamicScript) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		if !c.Dynamic {
			return nil
		}
		return []rules.Finding{rules.Flag(rule, c.Arg(len(c.Argv)-1), "%s script uses variables or command substitution%s: the commands it runs depend on values not shown", c.Base(), rules.Where(c))}
	})
}
