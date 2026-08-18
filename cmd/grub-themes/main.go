// Command grub-themes browses, validates and applies GRUB themes.
//
// Only `list` and `lint` are implemented so far. See AGENTS.md for the plan
// and for the constraints any new subcommand has to respect.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarbojitrana/grub-themes/internal/lint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

const usage = `grub-themes - browse, validate and apply GRUB themes

  grub-themes list            list the themes in this repository
  grub-themes lint [ID...]    validate themes (no GRUB or reboot needed)
  grub-themes help

Not yet implemented: preview, apply, qemu. See AGENTS.md.
`

// themesRoot finds themes/ from the repo root or from anywhere inside it.
func themesRoot() string {
	if env := os.Getenv("GRUB_THEMES_DIR"); env != "" {
		return env
	}
	dir, err := os.Getwd()
	if err != nil {
		return "themes"
	}
	for {
		candidate := filepath.Join(dir, "themes")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "themes"
		}
		dir = parent
	}
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(2)
	}

	switch args[0] {
	case "list":
		os.Exit(cmdList())
	case "lint":
		os.Exit(cmdLint(args[1:]))
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		fmt.Print(usage)
		os.Exit(2)
	}
}

func cmdList() int {
	themes, err := theme.Discover(themesRoot())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(themes) == 0 {
		fmt.Println("no themes found")
		return 0
	}
	for _, t := range themes {
		m := t.Manifest.Theme
		fmt.Printf("  %-14s %-22s %s\n", m.ID, m.Name, m.Description)
	}
	return 0
}

func cmdLint(ids []string) int {
	themes, err := theme.Discover(themesRoot())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}

	failed := false
	checked := 0
	for _, t := range themes {
		if len(wanted) > 0 && !wanted[t.Manifest.Theme.ID] {
			continue
		}
		checked++
		res := lint.Check(t)
		if len(res.Findings) == 0 {
			fmt.Printf("  ok    %s\n", t.Manifest.Theme.ID)
			continue
		}
		fmt.Printf("  %s\n", t.Manifest.Theme.ID)
		for _, f := range res.Findings {
			fmt.Printf("    %-5s %s: %s\n", f.Severity, f.File, f.Message)
			if f.Hint != "" {
				fmt.Printf("          %s\n", f.Hint)
			}
		}
		if res.HasErrors() {
			failed = true
		}
	}

	if checked == 0 {
		fmt.Fprintln(os.Stderr, "no matching themes")
		return 1
	}
	if failed {
		return 1
	}
	return 0
}
