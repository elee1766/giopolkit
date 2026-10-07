package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathCandidates(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "s.sh"), nil, 0o644)
	got := PathCandidates("s.sh", d)
	if len(got) != 1 || got[0] != filepath.Join(d, "s.sh") {
		t.Fatalf("%v", got)
	}
	got = PathCandidates("--config=/etc/x", "/")
	if len(got) != 1 || got[0] != "/etc/x" {
		t.Fatalf("%v", got)
	}
	if got := PathCandidates("-y", d); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestScriptFile(t *testing.T) {
	cases := map[string][]string{
		"check.sh": {"/bin/sh", "check.sh"},
		"x.py":     {"/usr/bin/python3", "-u", "x.py"},
		"":         {"/bin/sh", "-c", "id"},
		" ":        {"/bin/bash", "-ec", "id"},
		"  ":       {"/usr/bin/python3", "-c", "print(1)"},
		"   ":      {"/bin/sh"},
	}
	for want, argv := range cases {
		if want != "check.sh" && want != "x.py" {
			want = ""
		}
		got := ""
		if i := ScriptFile(argv); i > 0 {
			got = argv[i]
		}
		if got != want {
			t.Errorf("ScriptFile(%q) = %q, want %q", argv, got, want)
		}
	}
}
