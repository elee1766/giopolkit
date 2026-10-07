package system

import (
	"strings"

	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/rules/cmdline"
)

// SecurityOff flags turning off firewalls, mandatory access control, and audit logging.
type SecurityOff struct{}

func (SecurityOff) Name() string { return "security-off" }

// securityUnits are services whose stop/disable/mask weakens the system.
var securityUnits = []string{"firewalld", "ufw", "nftables", "iptables", "apparmor", "auditd", "selinux", "fail2ban", "clamav", "clamd", "crowdstrike", "falcon-sensor", "osqueryd", "wazuh", "sshguard"}

func (rule SecurityOff) Check(r *rules.Request) []rules.Finding {
	return rules.EachCommand(r, func(c *cmdline.Command) []rules.Finding {
		args := strings.Join(c.Argv[1:], " ")
		flag := func(what string) []rules.Finding {
			return []rules.Finding{rules.Flag(rule, c.Arg(0), "%s%s", what, rules.Where(c))}
		}
		switch c.Base() {
		case "setenforce":
			if args == "0" || strings.EqualFold(args, "permissive") {
				return flag("setenforce turns SELinux enforcement off")
			}
		case "ufw":
			if strings.HasPrefix(args, "disable") {
				return flag("ufw disable turns the firewall off")
			}
		case "iptables", "ip6tables", "nft":
			if strings.Contains(args, "-F") || strings.Contains(args, "--flush") || strings.Contains(args, "flush ruleset") || strings.Contains(args, "-P INPUT ACCEPT") {
				return flag(c.Base() + " removes firewall rules")
			}
		case "aa-teardown", "aa-disable":
			return flag(c.Base() + " turns AppArmor profiles off")
		case "auditctl":
			if strings.Contains(args, "-e 0") || strings.Contains(args, "-D") {
				return flag("auditctl turns audit logging off")
			}
		case "systemctl", "service":
			verb := ""
			for _, a := range c.Argv[1:] {
				if !strings.HasPrefix(a, "-") {
					if verb == "" {
						verb = a
						continue
					}
					if verb != "stop" && verb != "disable" && verb != "mask" && verb != "kill" {
						break
					}
					for _, u := range securityUnits {
						if strings.TrimSuffix(a, ".service") == u {
							return flag(c.Base() + " " + verb + " " + a + " turns off a security service")
						}
					}
				}
			}
		}
		return nil
	})
}
