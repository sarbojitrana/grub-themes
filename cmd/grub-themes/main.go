// Command grub-themes browses, validates, builds and applies GRUB themes.
//
// No arguments gives the browser. Everything it does is also a subcommand,
// because half of this gets used over SSH and in CI. See AGENTS.md before
// changing `apply`.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarbojitrana/grub-themes/internal/assets"
	"github.com/sarbojitrana/grub-themes/internal/install"
	"github.com/sarbojitrana/grub-themes/internal/lint"
	"github.com/sarbojitrana/grub-themes/internal/preview"
	"github.com/sarbojitrana/grub-themes/internal/scaffold"
	"github.com/sarbojitrana/grub-themes/internal/theme"
	"github.com/sarbojitrana/grub-themes/internal/tui"
)

// Version is set at build time: -ldflags "-X main.Version=1.2.3".
var Version = "dev"

const usage = `grub-themes — browse, build and apply GRUB themes

  grub-themes                     browse the themes and apply one
  grub-themes list                list what is available
  grub-themes status              what GRUB is configured to use right now

  grub-themes new ID              scaffold a new theme of your own
  grub-themes build [ID...]       generate a theme's assets from theme.toml
  grub-themes lint [ID...]        validate (no GRUB, no reboot needed)
  grub-themes preview [ID...]     render theme.txt to a PNG

  grub-themes apply ID            install it and regenerate grub.cfg (needs root)
  grub-themes remove              take the current theme back out (needs root)

Common flags:
  --themes-dir DIR   look for themes here instead of the usual places
  --dry-run          apply/remove: print every step, change nothing

Themes are looked for in $GRUB_THEMES_DIR, then ./themes in a checkout, then
~/.local/share/grub-themes/themes, then /usr/share/grub-themes/themes.
`

// themesDir overrides the search paths; empty means use them.
var themesDir string

func main() {
	args := os.Args[1:]
	args = extractThemesDir(args)

	if len(args) == 0 {
		os.Exit(cmdBrowse())
	}

	switch args[0] {
	case "list":
		os.Exit(cmdList())
	case "status":
		os.Exit(cmdStatus())
	case "lint":
		os.Exit(cmdLint(args[1:]))
	case "build":
		os.Exit(cmdBuild(args[1:]))
	case "preview":
		os.Exit(cmdPreview(args[1:]))
	case "new":
		os.Exit(cmdNew(args[1:]))
	case "apply":
		os.Exit(cmdApply(args[1:]))
	case "remove", "uninstall":
		os.Exit(cmdRemove(args[1:]))
	case "browse":
		os.Exit(cmdBrowse())
	case "version", "--version", "-v":
		fmt.Println("grub-themes", Version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		fmt.Print(usage)
		os.Exit(2)
	}
}

// extractThemesDir handles --themes-dir before the subcommand flag sets run.
func extractThemesDir(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--themes-dir" || args[i] == "-themes-dir":
			if i+1 < len(args) {
				themesDir = args[i+1]
				i++
			}
		case strings.HasPrefix(args[i], "--themes-dir="):
			themesDir = strings.TrimPrefix(args[i], "--themes-dir=")
		default:
			out = append(out, args[i])
		}
	}
	return out
}

func searchPaths() []string {
	if themesDir != "" {
		return []string{themesDir}
	}
	return theme.SearchPaths()
}

func allThemes() ([]theme.Theme, error) {
	themes, err := theme.DiscoverAll(searchPaths())
	if err != nil {
		return nil, err
	}
	if len(themes) == 0 {
		return nil, fmt.Errorf("no themes found in %s", strings.Join(searchPaths(), ", "))
	}
	return themes, nil
}

// selectThemes returns the themes matching ids, or all of them when ids is empty.
func selectThemes(ids []string) ([]theme.Theme, error) {
	all, err := allThemes()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return all, nil
	}
	var out []theme.Theme
	for _, id := range ids {
		found := false
		for _, t := range all {
			if t.Manifest.Theme.ID == id {
				out = append(out, t)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no such theme: %s", id)
		}
	}
	return out, nil
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

// ------------------------------------------------------------------ browse

func cmdBrowse() int {
	themes, err := theme.DiscoverAll(searchPaths())
	if err != nil {
		return fail(err)
	}
	if err := tui.Run(themes, themesDir); err != nil {
		return fail(err)
	}
	return 0
}

// -------------------------------------------------------------------- list

func cmdList() int {
	themes, err := allThemes()
	if err != nil {
		return fail(err)
	}
	st, _ := install.Read()
	for _, t := range themes {
		m := t.Manifest.Theme
		mark := " "
		if m.ID == st.Active {
			mark = "*"
		}
		fmt.Printf(" %s %-14s %-24s %s\n", mark, m.ID, m.Name, m.Description)
	}
	if st.Active != "" {
		fmt.Printf("\n * currently applied\n")
	}
	return 0
}

func cmdStatus() int {
	st, err := install.Read()
	if err != nil {
		return fail(err)
	}
	fmt.Printf("  grub.cfg    %s\n", st.Layout.GrubCfg)
	fmt.Printf("  generator   %s\n", st.Layout.MkConfig)
	fmt.Printf("  defaults    %s\n", st.Layout.Defaults)
	fmt.Printf("  themes dir  %s\n", st.Layout.ThemesDir)
	if st.Active == "" {
		fmt.Printf("  applied     (none)\n")
	} else {
		fmt.Printf("  applied     %s  ->  %s\n", st.Active, st.ActivePath)
	}
	if len(st.Installed) > 0 {
		fmt.Printf("  installed   %s\n", strings.Join(st.Installed, ", "))
	}
	if st.ConsoleForce {
		fmt.Printf("\n  ! GRUB_TERMINAL_OUTPUT=console is set in %s.\n", st.Layout.Defaults)
		fmt.Printf("    No theme can render until that is commented out; applying a theme does it for you.\n")
	}
	return 0
}

// -------------------------------------------------------------------- lint

func cmdLint(ids []string) int {
	themes, err := selectThemes(ids)
	if err != nil {
		return fail(err)
	}
	failed := false
	for _, t := range themes {
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
	if failed {
		return 1
	}
	return 0
}

// ------------------------------------------------------------------- build

func cmdBuild(ids []string) int {
	themes, err := selectThemes(ids)
	if err != nil {
		return fail(err)
	}
	for _, t := range themes {
		if t.Manifest.Assets == nil {
			fmt.Printf("  skip  %s (no [assets] section in theme.toml)\n", t.Manifest.Theme.ID)
			continue
		}
		rep, err := assets.Build(t)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", t.Manifest.Theme.ID, err))
		}
		fmt.Printf("  built %s: %d files (every PNG colour-type 6, depth 8)\n",
			t.Manifest.Theme.ID, len(rep.Written))
		for _, f := range rep.Fonts {
			// This name, not the filename, is what theme.txt must reference.
			fmt.Printf("        %-24s -> %q\n", f.File, f.Name)
		}
		for _, n := range rep.Notes {
			fmt.Printf("        note: %s\n", n)
		}
	}
	return 0
}

// ----------------------------------------------------------------- preview

func cmdPreview(args []string) int {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	out := fs.String("o", "", "write to this path instead of the theme's preview.png")
	selected := fs.Int("selected", 0, "which menu entry to draw as selected")
	fs.Parse(reorder(fs, args))

	themes, err := selectThemes(fs.Args())
	if err != nil {
		return fail(err)
	}
	if *out != "" && len(themes) != 1 {
		return fail(fmt.Errorf("-o only makes sense with exactly one theme"))
	}
	for _, t := range themes {
		dst := *out
		if dst == "" {
			dst = filepath.Join(t.Dir, "preview.png")
		}
		if err := preview.Write(t, dst, preview.Options{Selected: *selected}); err != nil {
			return fail(fmt.Errorf("%s: %w", t.Manifest.Theme.ID, err))
		}
		fmt.Printf("  wrote %s\n", dst)
	}
	return 0
}

// --------------------------------------------------------------------- new

func cmdNew(args []string) int {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	name := fs.String("name", "", "display name (defaults to the id, capitalised)")
	accent := fs.String("accent", "#00d9ff", "accent colour")
	github := fs.String("github", "", "your GitHub username")
	dir := fs.String("dir", "", "where to create it (defaults to ~/.local/share/grub-themes/themes)")
	fs.Parse(reorder(fs, args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: grub-themes new ID")
		return 2
	}
	id := fs.Arg(0)

	target := *dir
	if target == "" {
		target = theme.UserDir()
		// In a checkout, scaffold into the repository: that is where a theme meant
		// for a pull request belongs.
		if paths := theme.SearchPaths(); len(paths) > 0 && strings.HasSuffix(paths[0], "themes") {
			if _, err := os.Stat(filepath.Join(paths[0], "..", "go.mod")); err == nil {
				target = paths[0]
			}
		}
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fail(err)
	}

	dest, err := scaffold.New(target, id, scaffold.Params{
		Name:   *name,
		Accent: *accent,
		GitHub: *github,
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("  created %s\n\n", dest)

	// Build straight away, so this is a working theme and not placeholders.
	t, err := theme.Load(dest)
	if err != nil {
		return fail(err)
	}
	rep, err := assets.Build(t)
	if err != nil {
		return fail(err)
	}
	for _, f := range rep.Fonts {
		fmt.Printf("  %-24s -> %q\n", f.File, f.Name)
	}
	for _, n := range rep.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	if err := preview.Write(t, filepath.Join(dest, "preview.png"), preview.Options{}); err != nil {
		fmt.Fprintf(os.Stderr, "  preview: %v\n", err)
	}

	res := lint.Check(t)
	fmt.Println()
	if res.HasErrors() {
		for _, f := range res.Findings {
			fmt.Printf("    %-5s %s: %s\n", f.Severity, f.File, f.Message)
		}
	}
	fmt.Printf(`  Next:
    $EDITOR %s/theme.toml            colours and the selection style
    $EDITOR %s/tools/background.svg  the art
    grub-themes build %s             regenerate everything
    grub-themes preview %s           see it, no reboot needed
    grub-themes apply %s             install it

  To contribute it back, copy the directory into themes/ in a fork of
  https://github.com/sarbojitrana/grub-themes and open a pull request.
  See docs/build-your-own-theme.md.
`, dest, dest, id, id, id)
	return 0
}

// ------------------------------------------------------------ apply/remove

func cmdApply(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	opt := installFlags(fs)
	pause := fs.Bool("pause", false, "wait for Enter before returning (used by the browser)")
	fs.Parse(reorder(fs, args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: grub-themes apply ID")
		return 2
	}
	themes, err := selectThemes([]string{fs.Arg(0)})
	if err != nil {
		return waitThen(*pause, fail(err))
	}
	if os.Geteuid() != 0 && !opt.DryRun {
		exe, _ := os.Executable()
		if exe == "" {
			exe = "grub-themes"
		}
		fmt.Fprintf(os.Stderr, `applying a theme edits /etc/default/grub and regenerates grub.cfg, so it needs root:

    sudo %s apply %s --themes-dir %s

Or run "grub-themes" and press enter on the theme, which asks for the password
for you. Add --dry-run to see every step without changing anything.
`, exe, fs.Arg(0), filepath.Dir(themes[0].Dir))
		return waitThen(*pause, 1)
	}
	if res := lint.Check(themes[0]); res.HasErrors() {
		fmt.Fprintf(os.Stderr, "refusing to apply %s: it does not pass lint\n", fs.Arg(0))
		for _, f := range res.Findings {
			fmt.Fprintf(os.Stderr, "    %-5s %s: %s\n", f.Severity, f.File, f.Message)
		}
		return waitThen(*pause, 1)
	}
	if err := install.Apply(themes[0], *opt); err != nil {
		return waitThen(*pause, fail(err))
	}
	fmt.Println("\n  Done — reboot to see it.")
	return waitThen(*pause, 0)
}

func cmdRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	opt := installFlags(fs)
	pause := fs.Bool("pause", false, "wait for Enter before returning (used by the browser)")
	fs.Parse(reorder(fs, args))

	if err := install.Remove(*opt); err != nil {
		return waitThen(*pause, fail(err))
	}
	return waitThen(*pause, 0)
}

// reorder moves flags in front of positional arguments.
//
// reorder moves flags ahead of positional arguments. Go's flag package stops
// at the first non-flag word, so `apply gotham --dry-run` would otherwise
// ignore the dry run -- on a command that edits the boot configuration.
func reorder(fs *flag.FlagSet, args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if len(a) < 2 || !strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) { // this flag takes a value
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, rest...)
}

func installFlags(fs *flag.FlagSet) *install.Options {
	var opt install.Options
	fs.BoolVar(&opt.DryRun, "dry-run", false, "print every step, change nothing")
	fs.BoolVar(&opt.NoRegen, "no-regen", false, "install the files but do not regenerate grub.cfg")
	fs.BoolVar(&opt.BootThemes, "boot-themes", false, "install under /boot/grub/themes instead of /usr/share")
	return &opt
}

// waitThen holds the output on screen until the browser repaints over it.
func waitThen(pause bool, code int) int {
	if pause {
		fmt.Print("\n  press Enter to return to the browser ")
		fmt.Scanln()
	}
	return code
}
