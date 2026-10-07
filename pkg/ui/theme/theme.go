// Package theme holds the window palette and applies color overrides from config and Xresources.
package theme

import (
	"errors"
	"fmt"
	"image/color"
	"maps"
	"strconv"
	"strings"

	"github.com/elee1766/giopolkit/pkg/sys/xresources"
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

// Colors is the color section of the config file.
type Colors struct {
	// Xresources loads colors from the X resource database (xrdb -query) when true.
	Xresources bool `yaml:"xresources"`
	// XresourcesFile reads resources from this file instead of xrdb. Implies Xresources.
	XresourcesFile string `yaml:"xresources_file"`
	// XresourcesKeys overrides which resource feeds each palette key, e.g. {dim: color7}.
	XresourcesKeys map[string]string `yaml:"xresources_keys"`
	// Override sets palette keys. Applied after Xresources.
	Override map[string]string `yaml:"colors"`
}

// Apply applies c to p. Bad entries are skipped and reported together.
func (c Colors) Apply(p *Palette) error {
	var errs []error
	if c.Xresources || c.XresourcesFile != "" {
		res, err := xresources.Load(c.XresourcesFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("xresources: %w", err))
		}
		keys := maps.Clone(DefaultXresourcesKeys)
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
	for k, v := range c.Override {
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
