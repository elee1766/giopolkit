package fs

import (
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// SensitiveWrite flags commands that change files controlling logins, privileges, boot, and
// startup programs: /etc/sudoers, /etc/shadow, PAM, cron, systemd units, authorized_keys, and so
// on. Editing them is sometimes the point, so the finding says which file to check.
type SensitiveWrite struct{}

func (SensitiveWrite) Name() string { return "sensitive-write" }

// writers modify the files named in their non-option arguments. The value is the minimum argument
// position that is a destination (cp/mv/install: the last argument).
var writers = map[string]string{
	"tee": "all", "cp": "last", "mv": "last", "install": "last", "ln": "last", "rsync": "last",
	"dd": "of", "truncate": "all", "rm": "all", "unlink": "all", "shred": "all", "chmod": "all",
	"chown": "all", "chgrp": "all", "chattr": "all", "setfacl": "all", "sed": "inplace", "perl": "inplace",
	"vi": "all", "vim": "all", "nvim": "all", "nano": "all", "emacs": "all", "ed": "all", "visudo": "all",
	"vipw": "all", "vigr": "all", "sudoedit": "all", "touch": "all", "mkdir": "all", "systemctl": "edit",
}

func (rule SensitiveWrite) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		var out []rules.Finding
		flag := func(arg int, path string) {
			if s := sensitivePath(clean(path, r.Cwd)); s != "" {
				out = append(out, rules.Flag(rule, arg, "%s changes %s%s", c.Base(), path, rules.Where(c)))
			}
		}
		for _, w := range c.Writes {
			flag(c.Arg(-1), w)
		}
		base := c.Base()
		switch base {
		case "visudo", "vipw", "vigr":
			out = append(out, rules.Flag(rule, c.Arg(0), "%s edits %s", base, map[string]string{"visudo": "/etc/sudoers", "vipw": "/etc/passwd", "vigr": "/etc/group"}[base]))
			return out
		case "useradd", "usermod", "userdel", "groupadd", "groupmod", "gpasswd", "passwd", "chpasswd", "adduser", "deluser":
			out = append(out, rules.Flag(rule, c.Arg(0), "%s changes accounts or groups%s", base, rules.Where(c)))
			return out
		}
		mode, ok := writers[base]
		if !ok {
			return out
		}
		var ops []int
		for i := 1; i < len(c.Argv); i++ {
			a := c.Argv[i]
			if base == "dd" {
				if v, ok := strings.CutPrefix(a, "of="); ok {
					flag(c.Arg(i), v)
				}
				continue
			}
			if !strings.HasPrefix(a, "-") {
				ops = append(ops, i)
			}
		}
		switch mode {
		case "last":
			if len(ops) > 0 {
				i := ops[len(ops)-1]
				flag(c.Arg(i), c.Argv[i])
			}
		case "inplace":
			if !containsPrefix(c.Argv, "-i") {
				return out
			}
			for _, i := range ops[1:] {
				flag(c.Arg(i), c.Argv[i])
			}
		case "edit":
			if len(ops) > 0 && c.Argv[ops[0]] == "edit" {
				out = append(out, rules.Flag(rule, c.Arg(ops[0]), "systemctl edit changes a systemd unit%s", rules.Where(c)))
			}
		default:
			for _, i := range ops {
				flag(c.Arg(i), c.Argv[i])
			}
		}
		return out
	})
}

func containsPrefix(argv []string, p string) bool {
	for _, a := range argv[1:] {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}
