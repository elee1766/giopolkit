// Package theme holds the window palette and loads overrides from
// $XDG_CONFIG_HOME/giopolkit/config.yaml and, optionally, Xresources.
package theme

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Palette is the set of colors the window uses.
type Palette struct {
	Bg    color.NRGBA // window background
	Fg    color.NRGBA // body text, deny button fill
	Dim   color.NRGBA // labels, pids, disabled text
	Line  color.NRGBA // borders on unflagged requests
	Red   color.NRGBA // flagged requests: borders, warnings, highlighted arguments, root
	Green color.NRGBA // success, non-root target user
}

// Names lists palette keys in config and Xresources order.
var Names = []string{"bg", "fg", "dim", "line", "red", "green"}

func Default() Palette {
	return Palette{
		Bg:    rgb(0x121212),
		Fg:    rgb(0xd4d4d4),
		Dim:   rgb(0x8a8a8a),
		Line:  rgb(0x333333),
		Red:   rgb(0xe06c75),
		Green: rgb(0x98c379),
	}
}

// DefaultXresourcesKeys maps palette keys to the resource read for them when no
// giopolkit.<key> resource is set.
var DefaultXresourcesKeys = map[string]string{
	"bg":    "background",
	"fg":    "foreground",
	"dim":   "color8",
	"line":  "color0",
	"red":   "color1",
	"green": "color2",
}

func (p *Palette) field(name string) *color.NRGBA {
	switch name {
	case "bg":
		return &p.Bg
	case "fg":
		return &p.Fg
	case "dim":
		return &p.Dim
	case "line":
		return &p.Line
	case "red":
		return &p.Red
	case "green":
		return &p.Green
	}
	return nil
}

// Config is the YAML config file.
type Config struct {
	// Xresources loads colors from the X resource database (xrdb -query) when true.
	Xresources bool `yaml:"xresources"`
	// XresourcesFile reads resources from this file instead of xrdb. Implies Xresources.
	XresourcesFile string `yaml:"xresources_file"`
	// XresourcesKeys overrides which resource feeds each palette key, e.g. {dim: color7}.
	XresourcesKeys map[string]string `yaml:"xresources_keys"`
	// Colors overrides palette keys. Applied after Xresources.
	Colors map[string]string `yaml:"colors"`
}

// Path returns the config file location.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "giopolkit", "config.yaml")
}

// Load builds the palette: defaults, then Xresources if enabled, then config colors.
// A missing config file is not an error. Other errors are returned with the best palette
// that could be built.
func Load(path string) (Palette, error) {
	p := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	return p, c.Apply(&p)
}

// Apply applies c to p. Bad entries are skipped and reported together.
func (c Config) Apply(p *Palette) error {
	var errs []error
	if c.Xresources || c.XresourcesFile != "" {
		res, err := readResources(c.XresourcesFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("xresources: %w", err))
		}
		keys := map[string]string{}
		for k, v := range DefaultXresourcesKeys {
			keys[k] = v
		}
		for k, v := range c.XresourcesKeys {
			if p.field(k) == nil {
				errs = append(errs, fmt.Errorf("xresources_keys: unknown color %q", k))
				continue
			}
			keys[k] = v
		}
		for _, k := range Names {
			v, ok := res["giopolkit."+k]
			if !ok {
				v, ok = res[keys[k]]
			}
			if !ok {
				continue
			}
			col, err := ParseColor(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("xresources %s: %w", k, err))
				continue
			}
			*p.field(k) = col
		}
	}
	for k, v := range c.Colors {
		f := p.field(k)
		if f == nil {
			errs = append(errs, fmt.Errorf("colors: unknown color %q", k))
			continue
		}
		col, err := ParseColor(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("colors.%s: %w", k, err))
			continue
		}
		*f = col
	}
	return errors.Join(errs...)
}

func readResources(file string) (map[string]string, error) {
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
		return ParseResources(f), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xrdb", "-query").Output()
	if err != nil {
		return nil, fmt.Errorf("xrdb -query: %w", err)
	}
	return ParseResources(strings.NewReader(string(out))), nil
}

// ParseResources reads "name: value" lines. Wildcard entries (*.color1, *color1) are stored under
// the bare name, giopolkit.x and giopolkit*x under "giopolkit.x". Entries for other applications
// are ignored.
func ParseResources(r io.Reader) map[string]string {
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

// ParseColor accepts #rgb, #rrggbb, #rrggbbaa, and X11 rgb:r/g/b with 1 to 4 hex digits per channel.
func ParseColor(s string) (color.NRGBA, error) {
	s = strings.TrimSpace(s)
	bad := fmt.Errorf("invalid color %q", s)
	if h, ok := strings.CutPrefix(s, "rgb:"); ok {
		parts := strings.Split(h, "/")
		if len(parts) != 3 {
			return color.NRGBA{}, bad
		}
		var c [3]uint8
		for i, part := range parts {
			if len(part) < 1 || len(part) > 4 {
				return color.NRGBA{}, bad
			}
			v, err := strconv.ParseUint(part, 16, 16)
			if err != nil {
				return color.NRGBA{}, bad
			}
			c[i] = uint8(v * 255 / (1<<(4*len(part)) - 1))
		}
		return color.NRGBA{R: c[0], G: c[1], B: c[2], A: 0xff}, nil
	}
	h, ok := strings.CutPrefix(s, "#")
	if !ok {
		return color.NRGBA{}, bad
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return color.NRGBA{}, bad
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, bad
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}
