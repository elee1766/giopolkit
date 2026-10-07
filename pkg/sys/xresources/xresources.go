// Package xresources reads the X resource database.
package xresources

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Load reads resources from file, or from xrdb -query when file is "".
func Load(file string) (map[string]string, error) {
	if file != "" {
		if strings.HasPrefix(file, "~/") {
			home, _ := os.UserHomeDir()
			file = filepath.Join(home, file[2:])
		}
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return Parse(f), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xrdb", "-query").Output()
	if err != nil {
		return nil, fmt.Errorf("xrdb -query: %w", err)
	}
	return Parse(strings.NewReader(string(out))), nil
}

// Parse reads "name: value" lines. Wildcard entries (*.color1, *color1) are stored under
// the bare name, giopolkit.x and giopolkit*x under "giopolkit.x". Entries for other applications
// are ignored.
func Parse(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '!' || line[0] == '#' {
			continue
		}
		name, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		switch {
		case strings.HasPrefix(name, "*"):
			name = strings.TrimLeft(name, "*.")
		case strings.HasPrefix(name, "giopolkit.") || strings.HasPrefix(name, "giopolkit*"):
			name = "giopolkit." + strings.TrimLeft(name[len("giopolkit"):], "*.")
		default:
			continue
		}
		out[name] = val
	}
	return out
}
