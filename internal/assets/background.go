package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// buildBackground renders the theme's vector source to background.png.
//
// Whatever the renderer produces is decoded and written out again through
// paint.Save, so the file that lands in the theme is colour-type 6 regardless
// of what rsvg-convert or ImageMagick felt like emitting.
//
// The rendered PNG is committed, so a machine without a converter is not an
// error: it is reported and skipped.
func buildBackground(t theme.Theme, b *theme.Background) (string, string, error) {
	if b == nil || b.Source == "" {
		return "", "", nil
	}
	src := filepath.Join(t.Dir, b.Source)
	if _, err := os.Stat(src); err != nil {
		return "", "", fmt.Errorf("assets.background.source: %w", err)
	}
	out := b.Out
	if out == "" {
		out = "background.png"
	}
	dst := filepath.Join(t.Dir, out)

	w, h := b.Width, b.Height
	if w == 0 || h == 0 {
		w, h = 1920, 1080
	}

	tmp, err := os.CreateTemp("", "grub-themes-bg-*.png")
	if err != nil {
		return "", "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	switch {
	case have("rsvg-convert"):
		err = run("rsvg-convert", "-w", strconv.Itoa(w), "-h", strconv.Itoa(h), src, "-o", tmp.Name())
	case have("magick"):
		err = run("magick", "-background", "none", src, "-resize",
			strconv.Itoa(w)+"x"+strconv.Itoa(h)+"!", tmp.Name())
	case have("convert"):
		err = run("convert", "-background", "none", src, "-resize",
			strconv.Itoa(w)+"x"+strconv.Itoa(h)+"!", tmp.Name())
	default:
		return "", "no SVG renderer found (install librsvg or ImageMagick), keeping the committed " + out, nil
	}
	if err != nil {
		return "", "", err
	}

	img, err := paint.Load(tmp.Name())
	if err != nil {
		return "", "", fmt.Errorf("reading rendered background: %w", err)
	}
	if err := paint.Save(dst, img); err != nil {
		return "", "", err
	}
	return dst, "", nil
}

func have(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w\n%s", name, err, b)
	}
	return nil
}
