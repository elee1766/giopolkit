package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	argv := []string{"/usr/bin/systemd-run", "--unit=backup-home", "--property=Nice=10", "/usr/bin/restic", "backup", "/home/a", "--repo", "/mnt/backup/restic", "--one-file-system"}
	var got []string
	for _, ln := range splitCommand(argv) {
		var parts []string
		for _, i := range ln.args {
			parts = append(parts, argv[i])
		}
		got = append(got, fmt.Sprintf("%d %s", ln.indent, strings.Join(parts, " ")))
	}
	want := []string{
		"0 /usr/bin/systemd-run --unit=backup-home",
		"2 --property=Nice=10",
		"2 /usr/bin/restic backup /home/a",
		"4 --repo /mnt/backup/restic",
		"4 --one-file-system",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
