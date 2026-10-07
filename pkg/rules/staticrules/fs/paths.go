package fs

import (
	"path/filepath"
	"strings"
)

// critical are directories whose recursive removal breaks the system or loses user data.
var critical = []string{"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/opt", "/proc", "/root", "/sbin", "/srv", "/sys", "/usr", "/var", "/var/lib", "/var/log"}

// sensitive are files and directories that control authentication, privileges, boot, and what
// runs at startup. Writing to them as root changes who can do what on the system.
var sensitive = []string{
	"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/gshadow", "/etc/sudoers", "/etc/sudoers.d",
	"/etc/doas.conf", "/etc/pam.d", "/etc/security", "/etc/polkit-1", "/usr/share/polkit-1",
	"/etc/ssh", "/root/.ssh", "/etc/ld.so.preload", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	"/etc/crontab", "/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly",
	"/etc/cron.monthly", "/var/spool/cron", "/etc/systemd/system", "/usr/lib/systemd/system",
	"/lib/systemd/system", "/etc/profile", "/etc/profile.d", "/etc/bash.bashrc", "/etc/environment",
	"/etc/rc.local", "/etc/modules-load.d", "/etc/modprobe.d", "/etc/udev/rules.d", "/boot",
	"/etc/default/grub", "/etc/fstab", "/etc/hosts", "/etc/resolv.conf", "/etc/nsswitch.conf",
}

// clean resolves p against cwd and cleans it. Relative paths with an unknown cwd stay relative.
func clean(p, cwd string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		return p
	}
	if !filepath.IsAbs(p) && filepath.IsAbs(cwd) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

// isCritical reports whether p is a critical directory, a home directory, or a glob directly in one.
func isCritical(p string) bool {
	dir, base := p, ""
	if strings.ContainsAny(filepath.Base(p), "*?[") {
		dir, base = filepath.Dir(p), filepath.Base(p)
	}
	for _, c := range critical {
		if dir == c {
			return true
		}
	}
	// /home/USER itself
	if d, ok := strings.CutPrefix(dir, "/home/"); ok && !strings.Contains(d, "/") && base == "" {
		return true
	}
	return p == "~" || p == "~/" || strings.HasPrefix(p, "~/*") || p == "$HOME" || p == "/*"
}

// sensitivePath returns the sensitive entry p is at or under, or "".
func sensitivePath(p string) string {
	for _, s := range sensitive {
		if p == s || strings.HasPrefix(p, s+"/") {
			return s
		}
	}
	if strings.Contains(p, "/.ssh/authorized_keys") {
		return "authorized_keys"
	}
	return ""
}
