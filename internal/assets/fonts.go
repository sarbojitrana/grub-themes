package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarbojitrana/grub-themes/internal/pf2"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// glyphRanges is what gets baked into every .pf2.
//
// The full font is roughly twenty times larger, and almost all of that is
// scripts no boot menu uses. This covers Latin (including accents), Greek,
// Cyrillic, punctuation, currency, arrows, box drawing and block elements --
// enough for real menu entries and for the arrow hints themes like to draw.
var glyphRanges = []string{
	"0x20-0x7e",     // ASCII
	"0xa0-0x24f",    // Latin-1 supplement, Latin extended A/B
	"0x370-0x4ff",   // Greek and Cyrillic
	"0x2010-0x205f", // general punctuation, dashes, quotes
	"0x20a0-0x20bf", // currency
	"0x2190-0x21ff", // arrows
	"0x2500-0x25ff", // box drawing, blocks, geometric shapes
}

// FontResult is one baked font: the file, and the name GRUB will match on.
type FontResult struct {
	File string
	Name string
}

// buildFonts runs grub-mkfont for each declared size.
//
// The .pf2 files are committed, so a missing grub-mkfont is not an error --
// most contributors never touch the fonts. It is reported and skipped.
func buildFonts(t theme.Theme, f *theme.Fonts) ([]FontResult, string, error) {
	if f == nil || (f.Regular == "" && f.Bold == "") {
		return nil, "", nil
	}
	if _, err := exec.LookPath("grub-mkfont"); err != nil {
		return nil, "grub-mkfont not found, keeping the committed .pf2 files (install grub to rebuild them)", nil
	}

	prefix := f.Prefix
	if prefix == "" {
		prefix = "font"
	}
	var out []FontResult
	bake := func(ttf string, sizes []int, boldTag string) error {
		if ttf == "" || len(sizes) == 0 {
			return nil
		}
		src, err := findFont(t.Dir, ttf)
		if err != nil {
			return err
		}
		for _, size := range sizes {
			name := prefix + boldTag + "-" + strconv.Itoa(size) + ".pf2"
			dst := filepath.Join(t.Dir, name)
			args := []string{"-s", strconv.Itoa(size)}
			for _, r := range glyphRanges {
				args = append(args, "-r", r)
			}
			args = append(args, "-o", dst, src)
			cmd := exec.Command("grub-mkfont", args...)
			// grub-mkfont warns about font features it does not implement on
			// nearly every modern font; that is not a problem for a bitmap
			// dump, so only failure is reported.
			if b, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("grub-mkfont %s: %w\n%s", name, err, b)
			}
			baked, err := pf2.Name(dst)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			out = append(out, FontResult{File: name, Name: baked})
		}
		return nil
	}
	if err := bake(f.Regular, f.Sizes, ""); err != nil {
		return nil, "", err
	}
	if err := bake(f.Bold, f.BoldSizes, "-bold"); err != nil {
		return nil, "", err
	}
	return out, "", nil
}

// findFont locates a TTF/OTF by filename: in the theme directory first, so a
// theme can ship its own, then in the usual system font locations.
func findFont(themeDir, name string) (string, error) {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		return "", fmt.Errorf("font not found: %s", name)
	}
	roots := []string{themeDir}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".local/share/fonts"), filepath.Join(home, ".fonts"))
	}
	roots = append(roots, "/usr/share/fonts", "/usr/local/share/fonts", "/run/current-system/sw/share/X11/fonts")

	want := strings.ToLower(name)
	for _, root := range roots {
		var found string
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || found != "" {
				return nil
			}
			if strings.ToLower(d.Name()) == want {
				found = path
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("font %q not found in the theme directory or the system font paths", name)
}
