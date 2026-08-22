// Package scaffold creates a new theme from a template compiled into the
// binary.
//
// The point is that adding a theme never requires reading another theme.
// Copying themes/jarvis used to be the documented route, and it dragged along
// decisions -- and build scripts -- that had nothing to do with the new theme.
package scaffold

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed template
var files embed.FS

// Params fill in the template.
type Params struct {
	ID     string
	Name   string
	Upper  string
	Author string
	GitHub string
	Accent string
}

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}$`)

// New writes a complete theme into dir/<id> and returns the path.
func New(dir, id string, p Params) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("theme id %q must be lowercase letters, digits and dashes", id)
	}
	p.ID = id
	if p.Name == "" {
		p.Name = strings.ToUpper(id[:1]) + id[1:]
	}
	p.Upper = strings.ToUpper(p.Name)
	if p.Author == "" {
		p.Author = guessAuthor()
	}
	if p.Accent == "" {
		p.Accent = "#00d9ff"
	}

	dest := filepath.Join(dir, id)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s already exists", dest)
	}

	err := renderTree("template", dest, p)
	if err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}

func renderTree(root, dest string, p Params) error {
	entries, err := files.ReadDir(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		src := root + "/" + e.Name()
		if e.IsDir() {
			if err := renderTree(src, filepath.Join(dest, e.Name()), p); err != nil {
				return err
			}
			continue
		}
		body, err := files.ReadFile(src)
		if err != nil {
			return err
		}
		tmpl, err := template.New(e.Name()).Parse(string(body))
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(dest, strings.TrimSuffix(e.Name(), ".tmpl")))
		if err != nil {
			return err
		}
		err = tmpl.Execute(out, p)
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// guessAuthor uses the git identity, which is nearly always the right answer
// and saves an interactive prompt.
func guessAuthor() string {
	out, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return "Your Name"
	}
	if name := strings.TrimSpace(string(out)); name != "" {
		return name
	}
	return "Your Name"
}
