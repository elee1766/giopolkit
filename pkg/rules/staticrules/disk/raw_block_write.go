package disk

import (
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
	"github.com/elee1766/giopolkit/pkg/sys/blockdev"
)

// RawBlockWrite flags commands that write directly to a block device, and says where it is mounted.
type RawBlockWrite struct{}

func (RawBlockWrite) Name() string { return "raw-block-write" }

// blockWriters write to every block device named in their arguments.
var blockWriters = map[string]bool{
	"mkfs": true, "mke2fs": true, "mkswap": true, "wipefs": true, "fdisk": true, "sfdisk": true,
	"cfdisk": true, "gdisk": true, "sgdisk": true, "parted": true, "blkdiscard": true, "shred": true,
	"cryptsetup": true, "pvcreate": true, "zpool": true, "flashrom": true,
}

func (rule RawBlockWrite) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		base := c.Base()
		isDD := base == "dd"
		if !isDD && !blockWriters[base] && !strings.HasPrefix(base, "mkfs.") {
			return nil
		}
		var out []rules.Finding
		for i := 1; i < len(c.Argv); i++ {
			a := c.Argv[i]
			path := a
			if isDD {
				v, ok := strings.CutPrefix(a, "of=")
				if !ok {
					continue
				}
				path = v
			} else if strings.HasPrefix(a, "-") {
				continue
			}
			dev, ok := blockdev.Lookup(path)
			if !ok {
				continue
			}
			out = append(out, rules.Flag(rule, c.Arg(i), "writes to block device %s%s", path, rules.Where(c)))
			if mps := blockdev.Mounts(dev); len(mps) > 0 {
				out = append(out, rules.Flag(rule, -1, "%s is mounted at %s", path, mps[0]))
			}
		}
		return out
	})
}
