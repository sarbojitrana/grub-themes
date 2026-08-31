// Package theme discovers and parses the themes under themes/.
package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// Manifest mirrors theme.toml.
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

	// Assets is what `grub-themes build` generates.
	Assets *Assets `toml:"assets"`
}

// Assets describes the pixmaps grub-themes generates for a theme.
type Assets struct {
	Selection struct {
		// pill | bar | underline | none
		Style  string `toml:"style"`
		Fill   string `toml:"fill"`
		Text   string `toml:"text"`
		Radius *int   `toml:"radius"`
		Height *int   `toml:"height"`
		Width  *int   `toml:"width"`
		// Shadow is an offset ghost behind the highlight.
		Shadow   string `toml:"shadow"`
		ShadowDX *int   `toml:"shadow_dx"`
		ShadowDY *int   `toml:"shadow_dy"`
		// Thickness is the rule height for style = "underline".
		Thickness *int `toml:"thickness"`
	} `toml:"selection"`

	TerminalBox struct {
		// Transparent unless you want a slab over the theme while the kernel loads.
		Fill string `toml:"fill"`
	} `toml:"terminal_box"`

	Progress struct {
		Track  string `toml:"track"`
		Fill   string `toml:"fill"`
		Height *int   `toml:"height"`
	} `toml:"progress"`

	Fonts *Fonts `toml:"fonts"`

	Background *Background `toml:"background"`
}

// Background points at the vector source for the desktop image.
type Background struct {
	Source string `toml:"source"` // e.g. "tools/background.svg"
	Out    string `toml:"out"`    // defaults to "background.png"
	Width  int    `toml:"width"`
	Height int    `toml:"height"`
}

// Fonts describes the .pf2 files `grub-themes build` bakes with grub-mkfont.
//
// GRUB matches the name inside the .pf2, not the filename.
type Fonts struct {
	// TTF/OTF filenames, found in the theme dir or the system font paths.
	Regular string `toml:"regular"`
	Bold    string `toml:"bold"`
	// Prefix names the output: <prefix>-<size>.pf2, <prefix>-bold-<size>.pf2.
	Prefix    string `toml:"prefix"`
	Sizes     []int  `toml:"sizes"`
	BoldSizes []int  `toml:"bold_sizes"`
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

// SearchPaths is where themes are looked for, in priority order:
//
//  1. $GRUB_THEMES_DIR, if set;
//  2. themes/ in the repository you are standing in, so a checkout works
//     without installing anything;
//  3. $XDG_DATA_HOME/grub-themes/themes (~/.local/share/...) -- where
//     `grub-themes new` puts a theme you are writing, so it shows up in the
//     browser immediately;
//  4. each of $XDG_DATA_DIRS (/usr/local/share and /usr/share by default) --
//     the themes shipped by the package.
//
// Earlier paths win when two directories define the same theme id.
func SearchPaths() []string {
	if env := os.Getenv("GRUB_THEMES_DIR"); env != "" {
		return filepath.SplitList(env)
	}
	var paths []string
	if repo := repoThemes(); repo != "" {
		paths = append(paths, repo)
	}
	paths = append(paths, UserDir())
	for _, dir := range dataDirs() {
		paths = append(paths, filepath.Join(dir, "grub-themes", "themes"))
	}
	return paths
}

// UserDir is where a theme you are writing lives.
func UserDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "grub-themes", "themes")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "themes"
	}
	return filepath.Join(home, ".local", "share", "grub-themes", "themes")
}

// dataDirs is $XDG_DATA_DIRS, so /usr/local and /usr both work.
func dataDirs() []string {
	if env := os.Getenv("XDG_DATA_DIRS"); env != "" {
		return filepath.SplitList(env)
	}
	return []string{"/usr/local/share", "/usr/share"}
}

// repoThemes walks up looking for a themes/ directory with themes in it.
func repoThemes() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "themes")
		if matches, _ := filepath.Glob(filepath.Join(candidate, "*", "theme.toml")); len(matches) > 0 {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// DiscoverAll finds the themes in every search path, first definition winning.
func DiscoverAll(roots []string) ([]Theme, error) {
	seen := map[string]bool{}
	var out []Theme
	for _, root := range roots {
		found, err := Discover(root)
		if err != nil {
			continue // a missing search path is normal, not an error
		}
		for _, t := range found {
			id := t.Manifest.Theme.ID
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Manifest.Theme.ID < out[j].Manifest.Theme.ID
	})
	return out, nil
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
