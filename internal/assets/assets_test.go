package assets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

const manifest = `
[theme]
id = "sample"
name = "Sample"
version = "1.0.0"
license = "MIT"
[author]
name = "Tester"

[assets.selection]
style     = "pill"
fill      = "#00d9ff"
text      = "#05202a"
radius    = 10
height    = 44
shadow    = "#ff2e63"
shadow_dx = -6
shadow_dy = 4

[assets.terminal_box]
fill = "transparent"

[assets.progress]
track = "#12202ad9"
fill  = "#00d9ff"
`

func TestBuildProducesDecodableSlices(t *testing.T) {
	dir := t.TempDir()
	var tm theme.Theme
	tm.Dir = dir
	if _, err := toml.Decode(manifest, &tm.Manifest); err != nil {
		t.Fatal(err)
	}

	rep, err := Build(tm)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Written) != 14 {
		t.Errorf("wrote %d files, want 14 (3 selection + 9 box + 2 progress)", len(rep.Written))
	}

	// The whole point: every one has to be colour-type 6.
	for _, p := range rep.Written {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if b[25] != 6 || b[24] != 8 {
			t.Errorf("%s: colour-type %d depth %d, want 6/8", filepath.Base(p), b[25], b[24])
		}
	}

	// Matching heights keep GRUB from scaling the highlight.
	for _, name := range []string{"select_w.png", "select_c.png", "select_e.png"} {
		img, err := paint.Load(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if h := img.Bounds().Dy(); h != 44 {
			t.Errorf("%s is %d tall, want 44", name, h)
		}
	}

	// Transparency is what stops a black slab during boot.
	box, err := paint.Load(filepath.Join(dir, "terminal_box_c.png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := box.At(4, 4).RGBA(); a != 0 {
		t.Errorf("terminal box centre alpha = %d, want 0", a)
	}
}

func TestBuildRejectsUnknownStyle(t *testing.T) {
	var tm theme.Theme
	tm.Dir = t.TempDir()
	if _, err := toml.Decode(manifest, &tm.Manifest); err != nil {
		t.Fatal(err)
	}
	tm.Manifest.Assets.Selection.Style = "swoosh"
	if _, err := Build(tm); err == nil {
		t.Fatal("expected an error for an unknown selection style")
	}
}
