package pf2

import (
	"path/filepath"
	"testing"
)

// The JARVIS fonts are the reference: every field checked here is one GRUB
// matches on, and a mismatch is a silent failure at boot.
func TestParseJarvisFonts(t *testing.T) {
	paths, err := filepath.Glob("../../themes/jarvis/*.pf2")
	if err != nil || len(paths) == 0 {
		t.Skip("no fonts to parse")
	}
	for _, p := range paths {
		f, err := Parse(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if f.Name == "" {
			t.Errorf("%s: no NAME section", p)
		}
		if f.MaxHeight == 0 || f.Ascent == 0 {
			t.Errorf("%s: no metrics (maxh=%d asc=%d)", p, f.MaxHeight, f.Ascent)
		}
		if g, _ := f.Glyph('A'); g == nil || g.Width == 0 {
			t.Errorf("%s: no glyph for 'A'", p)
		}
		if w := f.Measure("Arch Linux"); w <= 0 {
			t.Errorf("%s: Measure returned %d", p, w)
		}
		t.Logf("%-28s %-32q ptsz=%2d maxh=%2d asc=%2d desc=%d measure=%d",
			filepath.Base(p), f.Name, f.PointSize, f.MaxHeight, f.Ascent, f.Descent, f.Measure("Arch Linux"))
	}
}
