// Package theme discovers and parses the themes under themes/.
package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// Manifest mirrors theme.toml. See themes/jarvis/theme.toml for a worked
// example and CONTRIBUTING.md for what each field is for.
type Manifest struct {
	Theme struct {
		ID          string   `toml:"id"`
		Name        string   `toml:"name"`
		Description string   `toml:"description"`
		Version     string   `toml:"version"`
		License     string   `toml:"license"`
		Tags        []string `toml:"tags"`
	} `toml:"theme"`

	Author struct {
		Name   string `toml:"name"`
		GitHub string `toml:"github"`
	} `toml:"author"`

	Files struct {
		Entry   string `toml:"entry"`
		Preview string `toml:"preview"`
	} `toml:"files"`

	Display struct {
		DesignedFor string `toml:"designed_for"`
		Aspect      string `toml:"aspect"`
	} `toml:"display"`

	Build struct {
		Background string `toml:"background"`
		Assets     string `toml:"assets"`
		Fonts      string `toml:"fonts"`
	} `toml:"build"`
}

// Theme is a manifest plus the directory it was loaded from.
type Theme struct {
	Dir      string
	Manifest Manifest
}

// EntryPath is the absolute path to the GRUB theme.txt.
func (t Theme) EntryPath() string {
	entry := t.Manifest.Files.Entry
	if entry == "" {
		entry = "theme.txt"
	}
	return filepath.Join(t.Dir, entry)
}

// PreviewPath is the absolute path to the preview image, or "" if unset.
func (t Theme) PreviewPath() string {
	if t.Manifest.Files.Preview == "" {
		return ""
	}
	return filepath.Join(t.Dir, t.Manifest.Files.Preview)
}

// Load reads a single theme directory.
func Load(dir string) (Theme, error) {
	var t Theme
	t.Dir = dir
	path := filepath.Join(dir, "theme.toml")
	if _, err := toml.DecodeFile(path, &t.Manifest); err != nil {
		return t, fmt.Errorf("%s: %w", path, err)
	}
	if t.Manifest.Theme.ID == "" {
		t.Manifest.Theme.ID = filepath.Base(dir)
	}
	return t, nil
}

// Discover finds every theme under root (normally "themes"), sorted by id.
func Discover(root string) ([]Theme, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", root, err)
	}
	var out []Theme
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "theme.toml")); err != nil {
			continue // not a theme directory
		}
		t, err := Load(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.Theme.ID < out[j].Manifest.Theme.ID
	})
	return out, nil
}
