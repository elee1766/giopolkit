package system

import (
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// KernelModule flags loading code into the kernel.
type KernelModule struct{}

func (KernelModule) Name() string { return "kernel-module" }

func (rule KernelModule) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		switch c.Base() {
		case "insmod", "kexec":
			return []rules.Finding{rules.Flag(rule, c.Arg(0), "%s loads code into the kernel%s", c.Base(), rules.Where(c))}
		case "modprobe":
			for _, a := range c.Argv[1:] {
				if a == "-r" || a == "--remove" {
					return nil
				}
			}
			return []rules.Finding{rules.Flag(rule, c.Arg(0), "modprobe loads a kernel module%s", rules.Where(c))}
		}
		return nil
	})
}
