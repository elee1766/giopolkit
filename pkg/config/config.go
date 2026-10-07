// Package config loads $XDG_CONFIG_HOME/giopolkit/config.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/elee1766/giopolkit/pkg/ui/theme"
)

// Config is the config file. Every field is optional.
type Config struct {
	Theme theme.Colors `yaml:",inline"`
}

// Path returns the default config file location.
func Path() string {
	dir, _ := os.UserConfigDir() // $XDG_CONFIG_HOME or ~/.config
	return filepath.Join(dir, "giopolkit", "config.yaml")
}

// Load reads path. A missing file gives the zero Config.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Palette builds the window palette from the config.
func (c Config) Palette() (theme.Palette, error) {
	p := theme.Default()
	return p, c.Theme.Apply(&p)
}
