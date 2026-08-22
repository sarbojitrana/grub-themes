package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempDefaults(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "grub")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdateDefaultsAddsThemeWhenMissing(t *testing.T) {
	p := tempDefaults(t, "GRUB_TIMEOUT=5\nGRUB_DEFAULT=0\n")
	if err := updateDefaults(p, "/usr/share/grub/themes/gotham", Options{Log: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if !strings.Contains(got, `GRUB_THEME="/usr/share/grub/themes/gotham/theme.txt"`) {
		t.Errorf("GRUB_THEME was not added:\n%s", got)
	}
	if !strings.Contains(got, "GRUB_TIMEOUT=5") {
		t.Error("the rest of the file must be left alone")
	}
}

func TestUpdateDefaultsReplacesExistingAndCommented(t *testing.T) {
	for _, existing := range []string{
		`GRUB_THEME="/usr/share/grub/themes/old/theme.txt"`,
		`#GRUB_THEME=`,
		`# GRUB_THEME="/boot/grub/themes/x/theme.txt"`,
	} {
		p := tempDefaults(t, existing+"\nGRUB_TIMEOUT=5\n")
		if err := updateDefaults(p, "/usr/share/grub/themes/new", Options{Log: &bytes.Buffer{}}); err != nil {
			t.Fatal(err)
		}
		got := read(t, p)
		if strings.Count(got, "GRUB_THEME") != 1 {
			t.Errorf("expected exactly one GRUB_THEME line, got:\n%s", got)
		}
		if !strings.Contains(got, "themes/new/theme.txt") {
			t.Errorf("theme not repointed from %q:\n%s", existing, got)
		}
	}
}

// GRUB_TERMINAL_OUTPUT=console disables graphics entirely: it is the single
// most common reason a theme appears to do nothing.
func TestUpdateDefaultsDisablesConsoleOutput(t *testing.T) {
	p := tempDefaults(t, "GRUB_TERMINAL_OUTPUT=console\n")
	var log bytes.Buffer
	if err := updateDefaults(p, "/usr/share/grub/themes/x", Options{Log: &log}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if !strings.Contains(got, "#GRUB_TERMINAL_OUTPUT=console") {
		t.Errorf("console output was not commented out:\n%s", got)
	}
	if !strings.Contains(log.String(), "gfxterm") {
		t.Error("the change should be reported, not made silently")
	}
}

func TestActiveThemeAndConsoleDetection(t *testing.T) {
	p := tempDefaults(t, "GRUB_THEME=\"/usr/share/grub/themes/arachne/theme.txt\"\nGRUB_TERMINAL=console\n")
	id, err := activeTheme(p)
	if err != nil {
		t.Fatal(err)
	}
	if id != "arachne" {
		t.Errorf("active theme = %q, want arachne", id)
	}
	if !consoleForced(p) {
		t.Error("GRUB_TERMINAL=console should count as forcing the console")
	}
}

func TestCommentThemeLeavesNothingActive(t *testing.T) {
	p := tempDefaults(t, "GRUB_THEME=\"/usr/share/grub/themes/x/theme.txt\"\n")
	if err := commentTheme(p); err != nil {
		t.Fatal(err)
	}
	if id, _ := activeTheme(p); id != "" {
		t.Errorf("still active after removal: %q", id)
	}
}

// The art sources are for contributors, not for the boot loader.
func TestCopyDirSkipsTools(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "tools"), 0o755)
	os.WriteFile(filepath.Join(src, "theme.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(src, "tools", "background.svg"), []byte("<svg/>"), 0o644)

	dst := filepath.Join(t.TempDir(), "out")
	if err := copyDir(src, dst, map[string]bool{"tools": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "theme.txt")); err != nil {
		t.Error("theme.txt should have been copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "tools")); !os.IsNotExist(err) {
		t.Error("tools/ should have been skipped")
	}
}

// The verification regexes are what stand between a theme and someone's lost
// recovery entry, so they are pinned here.
func TestConfigProbes(t *testing.T) {
	cfg := []byte(`menuentry 'Arch Linux' {
}
menuentry 'Windows Boot Manager' {
}
source /boot/grub/custom.cfg
### BEGIN /etc/grub.d/15_uki ###
`)
	if n := len(reMenuEntry.FindAll(cfg, -1)); n != 2 {
		t.Errorf("menu entries counted = %d, want 2", n)
	}
	if !reCustomInclu.Match(cfg) {
		t.Error("custom.cfg include not detected")
	}
	if !reUKI.Match(cfg) {
		t.Error("UKI entries not detected")
	}
}
