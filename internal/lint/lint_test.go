package lint

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// writeTheme lays out a minimal theme on disk and loads it.
func writeTheme(t *testing.T, manifest, entry string) theme.Theme {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sample")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("theme.toml", manifest)
	write("theme.txt", entry)

	var tm theme.Theme
	tm.Dir = dir
	if _, err := toml.Decode(manifest, &tm.Manifest); err != nil {
		t.Fatal(err)
	}
	if tm.Manifest.Theme.ID == "" {
		tm.Manifest.Theme.ID = "sample"
	}
	return tm
}

func findings(r Result, want string) *Finding {
	for i := range r.Findings {
		if strings.Contains(r.Findings[i].Message, want) {
			return &r.Findings[i]
		}
	}
	return nil
}

const goodManifest = `
[theme]
id = "sample"
name = "Sample"
version = "1.0.0"
license = "MIT"
[author]
name = "Tester"
[files]
entry = "theme.txt"
`

// A palette PNG is the classic silent failure: GRUB drops it and the selected
// entry looks blanked out rather than highlighted.
func TestRejectsWrongPNGColourType(t *testing.T) {
	tm := writeTheme(t, goodManifest, "desktop-image: \"background.png\"\n")

	// Fully opaque, so Go's encoder writes colour-type 2 rather than 6.
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	f, err := os.Create(filepath.Join(tm.Dir, "background.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res := Check(tm)
	if !res.HasErrors() {
		t.Fatalf("expected an error, got %v", res.Findings)
	}
	if findings(res, "colour-type") == nil {
		t.Errorf("expected a colour-type finding, got %v", res.Findings)
	}
}

func TestAcceptsColourType6(t *testing.T) {
	tm := writeTheme(t, goodManifest, "desktop-image: \"background.png\"\n")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 200}) // has alpha
		}
	}
	f, _ := os.Create(filepath.Join(tm.Dir, "background.png"))
	png.Encode(f, img)
	f.Close()

	for _, fnd := range Check(tm).Findings {
		if fnd.Severity == Error {
			t.Errorf("unexpected error: %s: %s", fnd.File, fnd.Message)
		}
	}
}

func TestFlagsUnreadableSelectionContrast(t *testing.T) {
	manifest := goodManifest + `
[assets.selection]
style = "pill"
fill  = "#2a2a2a"
text  = "#333333"
`
	tm := writeTheme(t, manifest, "title-text: \"\"\n")
	res := Check(tm)
	if findings(res, "contrast ratio") == nil {
		t.Fatalf("expected a contrast error, got %v", res.Findings)
	}
	if !res.HasErrors() {
		t.Error("a highlight nobody can read should be an error")
	}
}

func TestFlagsItemHeightMismatch(t *testing.T) {
	manifest := goodManifest + `
[assets.selection]
style  = "pill"
fill   = "#00d9ff"
text   = "#05202a"
height = 44
`
	entry := "+ boot_menu {\n  item_height = 60\n  selected_item_color = \"#05202a\"\n}\n"
	res := Check(writeTheme(t, manifest, entry))
	if f := findings(res, "item_height is 60"); f == nil {
		t.Fatalf("expected an item_height finding, got %v", res.Findings)
	} else if f.Severity != Warning {
		t.Error("a mismatched height is a warning, not an error")
	}
}

func TestFlagsColourDisagreement(t *testing.T) {
	manifest := goodManifest + `
[assets.selection]
style = "pill"
fill  = "#00d9ff"
text  = "#05202a"
`
	entry := "+ boot_menu {\n  selected_item_color = \"#ffffff\"\n}\n"
	res := Check(writeTheme(t, manifest, entry))
	if findings(res, "assets.selection.text") == nil {
		t.Fatalf("expected a disagreement finding, got %v", res.Findings)
	}
}

// GRUB matches fonts by the name inside the .pf2, never the filename.
func TestFlagsUnknownFont(t *testing.T) {
	entry := "+ boot_menu {\n  item_font = \"Nonexistent Font 20\"\n}\n"
	res := Check(writeTheme(t, goodManifest, entry))
	if findings(res, "is not baked into any .pf2") == nil {
		t.Fatalf("expected a font finding, got %v", res.Findings)
	}
}

// Every theme in the repository must stay clean: this is the check CI runs.
func TestShippedThemesPass(t *testing.T) {
	themes, err := theme.Discover("../../themes")
	if err != nil || len(themes) == 0 {
		t.Skip("no themes directory")
	}
	for _, tm := range themes {
		res := Check(tm)
		for _, f := range res.Findings {
			t.Errorf("%s: %s %s: %s", tm.Manifest.Theme.ID, f.Severity, f.File, f.Message)
		}
	}
}
