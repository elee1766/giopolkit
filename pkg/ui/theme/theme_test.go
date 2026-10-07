package theme

import (
	"image/color"
	"os"
	"strings"
	"testing"
)

func TestXresourcesFile(t *testing.T) {
	db := "*.background: #000000\n*color1: rgb:ff/00/00\nURxvt.color2: #00ff00\ngiopolkit*green: #0000ff\n! comment\n"
	f := t.TempDir() + "/Xresources"
	if err := writeFile(f, db); err != nil {
		t.Fatal(err)
	}
	p := Default()
	if err := (Colors{XresourcesFile: f, Override: map[string]string{"fg": "#abc"}}).Apply(&p); err != nil {
		t.Fatal(err)
	}
	want := map[string]color.NRGBA{
		"bg":    {0, 0, 0, 0xff},
		"red":   {0xff, 0, 0, 0xff},
		"green": {0, 0, 0xff, 0xff}, // giopolkit.green wins, URxvt.color2 is ignored
		"fg":    {0xaa, 0xbb, 0xcc, 0xff},
		"dim":   Default().Dim,
	}
	for k, c := range want {
		if got := *p.field(k); got != c {
			t.Errorf("%s = %v, want %v", k, got, c)
		}
	}
	if err := (Colors{Override: map[string]string{"red": "red", "x": "#fff"}}).Apply(&p); err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Errorf("bad config error = %v", err)
	}
}

func writeFile(path, s string) error { return os.WriteFile(path, []byte(s), 0o644) }
