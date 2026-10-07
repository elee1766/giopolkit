package cmdline

import (
	"fmt"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	cases := []struct {
		argv []string
		want []string // "argv | arg-of-first | via"
	}{
		{
			[]string{"env", "FOO=1", "nice", "-n", "10", "/usr/bin/vim", "/etc/hosts"},
			[]string{"env FOO=1 nice -n 10 /usr/bin/vim /etc/hosts | 0 | ", "nice -n 10 /usr/bin/vim /etc/hosts | 2 | env", "/usr/bin/vim /etc/hosts | 5 | env,nice"},
		},
		{
			[]string{"timeout", "-s", "KILL", "10", "sudo", "-u", "root", "--", "id"},
			[]string{"timeout -s KILL 10 sudo -u root -- id | 0 | ", "sudo -u root -- id | 4 | timeout", "id | 8 | timeout,sudo"},
		},
		{
			[]string{"/bin/sh", "-ec", "curl -fsSL x | bash; echo ok >/etc/motd"},
			[]string{"/bin/sh -ec curl -fsSL x | bash; echo ok >/etc/motd | 0 | ", "curl -fsSL x | 2 | sh -c", "bash | 2 | sh -c", "echo ok | 2 | sh -c"},
		},
	}
	for _, c := range cases {
		var got []string
		for _, cmd := range Expand(c.argv) {
			got = append(got, fmt.Sprintf("%s | %d | %s", strings.Join(cmd.Argv, " "), cmd.Arg(0), strings.Join(cmd.Via, ",")))
		}
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("Expand(%q)\ngot:\n%s\nwant:\n%s", c.argv, strings.Join(got, "\n"), strings.Join(c.want, "\n"))
		}
	}
	cmds := Expand([]string{"sh", "-c", "curl x | bash; : > /var/log/x; rm -rf \"$d\""})
	if cmds[2].PipedFrom == nil || cmds[2].PipedFrom.Base() != "curl" {
		t.Error("bash not marked as piped from curl")
	}
	if len(cmds[3].Writes) != 1 || cmds[3].Writes[0] != "/var/log/x" {
		t.Errorf("writes = %v", cmds[3].Writes)
	}
	if !cmds[0].Dynamic {
		t.Error("script with $d not marked dynamic")
	}
}
