// Package install applies and removes GRUB themes.
//
// This is the part of the project where a bug is genuinely dangerous: it edits
// the configuration your machine boots from. The contract, inherited from the
// shell installer this replaces and documented in AGENTS.md, is:
//
//   - probe for the layout rather than assuming it (grub vs grub2,
//     grub-mkconfig vs grub2-mkconfig);
//   - back up grub.cfg and /etc/default/grub before touching anything;
//   - before regenerating, record whether the current config had UKI entries,
//     whether it sourced custom.cfg, and how many menu entries it had;
//   - after regenerating, verify all three survived -- and if any did not,
//     restore the backup and fail.
//
// Someone else's recovery entry must not disappear because they tried a theme.
package install

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// BackupRoot is where grub.cfg and /etc/default/grub are copied before any
// change. Each run gets its own timestamped directory.
const BackupRoot = "/var/lib/grub-themes/backups"

// Options controls one apply or remove.
type Options struct {
	DryRun     bool      // print every step, change nothing
	NoRegen    bool      // install the files but leave grub.cfg alone
	BootThemes bool      // install under /boot/grub/themes instead of /usr/share
	Log        io.Writer // step-by-step progress; defaults to os.Stdout
}

func (o Options) log() io.Writer {
	if o.Log == nil {
		return os.Stdout
	}
	return o.Log
}

func (o Options) say(format string, args ...any) {
	fmt.Fprintf(o.log(), format+"\n", args...)
}

// Layout is where GRUB lives on this machine.
type Layout struct {
	GrubCfg   string // /boot/grub/grub.cfg
	MkConfig  string // grub-mkconfig or grub2-mkconfig
	Defaults  string // /etc/default/grub
	ThemesDir string // where themes are installed
}

// Detect probes for the layout. Distributions disagree about all of it:
// Debian and Arch use grub/, Fedora and openSUSE grub2/.
func Detect(opt Options) (Layout, error) {
	var l Layout

	for _, pattern := range []string{
		"/boot/grub/grub.cfg",
		"/boot/grub2/grub.cfg",
		"/boot/efi/EFI/*/grub.cfg",
	} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
				l.GrubCfg = m
				break
			}
		}
		if l.GrubCfg != "" {
			break
		}
	}
	if l.GrubCfg == "" {
		return l, fmt.Errorf("could not find grub.cfg -- is GRUB installed?")
	}

	for _, m := range []string{"grub-mkconfig", "grub2-mkconfig"} {
		if p, err := exec.LookPath(m); err == nil {
			l.MkConfig = p
			break
		}
	}
	if l.MkConfig == "" {
		return l, fmt.Errorf("neither grub-mkconfig nor grub2-mkconfig found")
	}

	l.Defaults = "/etc/default/grub"
	if _, err := os.Stat(l.Defaults); err != nil {
		return l, fmt.Errorf("%s not found", l.Defaults)
	}

	// /usr/share/grub/themes is the standard location and lives on the root
	// filesystem, which GRUB reads directly. /boot/grub/themes is preferred
	// only when /usr/share/grub does not exist or when asked for: on UEFI
	// systems /boot is often a small FAT ESP, and themes are megabytes.
	switch {
	case opt.BootThemes:
		l.ThemesDir = "/boot/grub/themes"
		if isDir("/boot/grub2") {
			l.ThemesDir = "/boot/grub2/themes"
		}
	case isDir("/usr/share/grub"):
		l.ThemesDir = "/usr/share/grub/themes"
	case isDir("/boot/grub2"):
		l.ThemesDir = "/boot/grub2/themes"
	default:
		l.ThemesDir = "/boot/grub/themes"
	}
	return l, nil
}

// Apply installs a theme and points GRUB at it.
func Apply(t theme.Theme, opt Options) error {
	l, err := Detect(opt)
	if err != nil {
		return err
	}
	if err := requireRoot(opt); err != nil {
		return err
	}

	id := t.Manifest.Theme.ID
	dest := filepath.Join(l.ThemesDir, id)

	opt.say("grub.cfg   %s", l.GrubCfg)
	opt.say("generator  %s", l.MkConfig)
	opt.say("themes     %s", l.ThemesDir)

	backup, err := makeBackup(l, id, opt)
	if err != nil {
		return err
	}
	opt.say("  backed up grub.cfg and %s to %s", l.Defaults, backup)

	if opt.DryRun {
		opt.say("  [dry-run] copy %s -> %s", t.Dir, dest)
	} else {
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
		// tools/ is the theme's own art sources; the boot loader has no use
		// for an SVG.
		if err := copyDir(t.Dir, dest, map[string]bool{"tools": true}); err != nil {
			return fmt.Errorf("copying theme: %w", err)
		}
		opt.say("  installed %s to %s", id, dest)
	}

	if err := updateDefaults(l.Defaults, dest, opt); err != nil {
		return err
	}

	if opt.NoRegen {
		opt.say("  --no-regen: run '%s -o %s' yourself to apply", l.MkConfig, l.GrubCfg)
		return nil
	}
	if opt.DryRun {
		opt.say("  [dry-run] %s -o %s", l.MkConfig, l.GrubCfg)
		return nil
	}
	return regenerate(l, backup, dest, opt)
}

// Remove takes the theme back out and restores the previous configuration.
func Remove(opt Options) error {
	l, err := Detect(opt)
	if err != nil {
		return err
	}
	if err := requireRoot(opt); err != nil {
		return err
	}

	active, _ := activeTheme(l.Defaults)
	if active == "" {
		opt.say("  no theme is set in %s", l.Defaults)
	}

	backup, err := makeBackup(l, "remove", opt)
	if err != nil {
		return err
	}
	opt.say("  backed up grub.cfg and %s to %s", l.Defaults, backup)

	if active != "" {
		dest := filepath.Join(l.ThemesDir, active)
		if opt.DryRun {
			opt.say("  [dry-run] rm -rf %s", dest)
		} else if err := os.RemoveAll(dest); err != nil {
			return err
		}
		opt.say("  removed %s", dest)
	}

	if opt.DryRun {
		opt.say("  [dry-run] comment out GRUB_THEME in %s", l.Defaults)
	} else if err := commentTheme(l.Defaults); err != nil {
		return err
	}
	opt.say("  GRUB_THEME commented out")

	if opt.NoRegen || opt.DryRun {
		opt.say("  run '%s -o %s' to apply", l.MkConfig, l.GrubCfg)
		return nil
	}
	return regenerate(l, backup, "", opt)
}

// Status describes the current state of the system's GRUB configuration.
type Status struct {
	Layout       Layout
	Active       string   // theme id, or ""
	ActivePath   string   // what GRUB_THEME points at
	Installed    []string // theme ids present in the themes directory
	ConsoleForce bool     // GRUB_TERMINAL(_OUTPUT)=console is set: no theme can render
}

// Read reports what GRUB is currently configured to do. It needs no privileges.
func Read() (Status, error) {
	var s Status
	l, err := Detect(Options{})
	if err != nil {
		return s, err
	}
	s.Layout = l
	s.ActivePath, _ = grubThemePath(l.Defaults)
	s.Active, _ = activeTheme(l.Defaults)
	s.ConsoleForce = consoleForced(l.Defaults)

	entries, _ := os.ReadDir(l.ThemesDir)
	for _, e := range entries {
		if e.IsDir() {
			s.Installed = append(s.Installed, e.Name())
		}
	}
	return s, nil
}

// ---------------------------------------------------------------- internals

func requireRoot(opt Options) error {
	if opt.DryRun || os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf("this needs root: re-run with sudo (or use --dry-run to see the steps)")
}

func makeBackup(l Layout, tag string, opt Options) (string, error) {
	dir := filepath.Join(BackupRoot, time.Now().Format("20060102-150405")+"-"+tag)
	if opt.DryRun {
		return dir + " (dry-run, not created)", nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := copyFile(l.GrubCfg, filepath.Join(dir, "grub.cfg")); err != nil {
		return "", err
	}
	if err := copyFile(l.Defaults, filepath.Join(dir, "default-grub")); err != nil {
		return "", err
	}
	return dir, nil
}

var (
	reThemeLine   = regexp.MustCompile(`(?m)^[#[:space:]]*GRUB_THEME\s*=.*$`)
	reThemeValue  = regexp.MustCompile(`(?m)^\s*GRUB_THEME\s*=\s*"?([^"\n]+)"?\s*$`)
	reConsoleOut  = regexp.MustCompile(`(?m)^\s*GRUB_TERMINAL(_OUTPUT)?\s*=\s*"?console"?\s*$`)
	reUKI         = regexp.MustCompile(`(?m)^\s*uki|15_uki`)
	reMenuEntry   = regexp.MustCompile(`(?m)^menuentry`)
	reCustomInclu = regexp.MustCompile(`custom\.cfg`)
)

// updateDefaults points GRUB_THEME at the installed theme, and disables the
// one setting that stops any theme rendering at all.
func updateDefaults(path, dest string, opt Options) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	line := fmt.Sprintf("GRUB_THEME=%q", filepath.Join(dest, "theme.txt"))

	if reThemeLine.MatchString(src) {
		src = reThemeLine.ReplaceAllString(src, line)
	} else {
		if !strings.HasSuffix(src, "\n") {
			src += "\n"
		}
		src += line + "\n"
	}

	// A theme needs a graphical terminal. GRUB_TERMINAL_OUTPUT=console turns
	// gfxterm off entirely, and is the single most common reason a GRUB theme
	// appears to "do nothing".
	if reConsoleOut.MatchString(src) {
		src = reConsoleOut.ReplaceAllStringFunc(src, func(m string) string { return "#" + m })
		opt.say("  commented out GRUB_TERMINAL_OUTPUT=console (themes need gfxterm)")
	}

	if opt.DryRun {
		opt.say("  [dry-run] set %s in %s", line, path)
		return nil
	}
	if err := writeFile(path, []byte(src), 0o644); err != nil {
		return err
	}
	opt.say("  GRUB_THEME set")
	return nil
}

func commentTheme(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := reThemeLine.ReplaceAllString(string(b), "#GRUB_THEME=")
	return writeFile(path, []byte(src), 0o644)
}

// regenerate rebuilds grub.cfg and proves the result still boots what the old
// one booted. Anything short of that restores the backup.
func regenerate(l Layout, backup, dest string, opt Options) error {
	before, err := os.ReadFile(filepath.Join(backup, "grub.cfg"))
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}
	hadUKI := reUKI.Match(before)
	hadCustom := reCustomInclu.Match(before)
	entriesBefore := len(reMenuEntry.FindAll(before, -1))

	opt.say("regenerating %s", l.GrubCfg)
	cmd := exec.Command(l.MkConfig, "-o", l.GrubCfg)
	out, runErr := cmd.CombinedOutput()
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			opt.say("    %s", line)
		}
	}
	if runErr != nil {
		restore(l, backup, opt)
		return fmt.Errorf("%s failed -- previous grub.cfg restored, boot unchanged: %w",
			filepath.Base(l.MkConfig), runErr)
	}

	after, err := os.ReadFile(l.GrubCfg)
	if err != nil {
		restore(l, backup, opt)
		return fmt.Errorf("cannot read the new grub.cfg -- previous one restored: %w", err)
	}

	var problems []string
	if hadUKI && !reUKI.Match(after) {
		problems = append(problems, "UKI entries disappeared")
	}
	if hadCustom && !reCustomInclu.Match(after) {
		problems = append(problems, "the custom.cfg include disappeared")
	}
	entriesAfter := len(reMenuEntry.FindAll(after, -1))
	if entriesAfter < entriesBefore {
		problems = append(problems,
			fmt.Sprintf("menu entries dropped from %d to %d", entriesBefore, entriesAfter))
	}
	if dest != "" && !strings.Contains(string(after), dest) {
		problems = append(problems, "the theme is not referenced in the new grub.cfg")
	}

	if len(problems) > 0 {
		restore(l, backup, opt)
		return fmt.Errorf("verification failed (%s) -- previous grub.cfg restored, boot unchanged",
			strings.Join(problems, "; "))
	}

	opt.say("  verified: %d menu entries, UKI and custom.cfg intact", entriesAfter)
	return nil
}

func restore(l Layout, backup string, opt Options) {
	if err := copyFile(filepath.Join(backup, "grub.cfg"), l.GrubCfg); err != nil {
		opt.say("  RESTORE FAILED: %v -- your backup is at %s", err, backup)
	}
	if err := copyFile(filepath.Join(backup, "default-grub"), l.Defaults); err != nil {
		opt.say("  RESTORE FAILED: %v -- your backup is at %s", err, backup)
	}
}

func grubThemePath(defaults string) (string, error) {
	b, err := os.ReadFile(defaults)
	if err != nil {
		return "", err
	}
	m := reThemeValue.FindSubmatch(b)
	if m == nil {
		return "", nil
	}
	return strings.TrimSpace(string(m[1])), nil
}

// activeTheme derives the theme id from the directory GRUB_THEME points into.
func activeTheme(defaults string) (string, error) {
	p, err := grubThemePath(defaults)
	if err != nil || p == "" {
		return "", err
	}
	return filepath.Base(filepath.Dir(p)), nil
}

func consoleForced(defaults string) bool {
	b, err := os.ReadFile(defaults)
	if err != nil {
		return false
	}
	return reConsoleOut.Match(b)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".grub-themes.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// copyDir copies a theme into place, world-readable: GRUB reads these files
// before any user exists.
func copyDir(src, dst string, skip map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if skip[strings.Split(rel, string(os.PathSeparator))[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		return os.Chmod(target, 0o644)
	})
}
