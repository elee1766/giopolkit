package inspect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanModify(t *testing.T) {
	uid := os.Getuid()
	if uid == 0 {
		t.Skip("run as a normal user")
	}
	g := map[uint32]bool{uint32(os.Getgid()): true}
	if mod, via := CanModify("/usr/bin/env", uid, g); mod {
		t.Fatalf("/usr/bin/env modifiable via %s", via)
	}
	d := t.TempDir()
	f := filepath.Join(d, "x")
	os.WriteFile(f, nil, 0o755)
	if mod, _ := CanModify(f, uid, g); !mod {
		t.Fatal("own file not modifiable")
	}
	// Our symlink in our dir pointing at a system binary: we can repoint it.
	l := filepath.Join(d, "l")
	os.Symlink("/usr/bin/env", l)
	if mod, _ := CanModify(l, uid, g); !mod {
		t.Fatal("own symlink not flagged")
	}
	// A root-owned symlink in a root-owned dir that points at a system binary: safe.
	if fi, err := os.Lstat("/bin"); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if mod, via := CanModify("/bin/sh", uid, g); mod {
			t.Fatalf("/bin/sh modifiable via %s", via)
		}
	}
}

func TestPathCandidates(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "s.sh"), nil, 0o644)
	got := pathCandidates("s.sh", d)
	if len(got) != 1 || got[0] != filepath.Join(d, "s.sh") {
		t.Fatalf("%v", got)
	}
	got = pathCandidates("--config=/etc/x", "/")
	if len(got) != 1 || got[0] != "/etc/x" {
		t.Fatalf("%v", got)
	}
	if got := pathCandidates("-y", d); len(got) != 0 {
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
		if i := scriptFile(argv); i > 0 {
			got = argv[i]
		}
		if got != want {
			t.Errorf("scriptFile(%q) = %q, want %q", argv, got, want)
		}
	}
}
