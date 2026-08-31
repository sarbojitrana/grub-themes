package tui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sarbojitrana/grub-themes/internal/paint"
)

// renderImage draws an image into a block of terminal cells.
//
// Each cell holds two pixels: the top half is the foreground of "▀", the
// bottom its background. Cells are about twice as tall as they are wide, so
// this comes out roughly square.
func renderImage(img image.Image, cols, rows int) string {
	if cols < 4 || rows < 2 {
		return ""
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}

	// Fit the image inside the cell block, preserving its aspect ratio.
	aspect := float64(b.Dy()) / float64(b.Dx())
	w := cols
	h := int(float64(w) * aspect / 2)
	if h > rows {
		h = rows
		w = int(float64(h) * 2 / aspect)
	}
	if w < 2 || h < 1 {
		return ""
	}

	small := paint.Scale(img, w, h*2)
	// lipgloss owns the profile so tests can pin it and 256-colour degrades.
	profile := lipgloss.ColorProfile()

	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			top := small.NRGBAAt(x, y*2)
			bottom := small.NRGBAAt(x, y*2+1)
			sb.WriteString(cell(profile, top, bottom))
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func cell(profile termenv.Profile, top, bottom color.NRGBA) string {
	fg := profile.Color(hex(top))
	bg := profile.Color(hex(bottom))
	s := termenv.String("▀")
	if fg != nil {
		s = s.Foreground(fg)
	}
	if bg != nil {
		s = s.Background(bg)
	}
	return s.String()
}

func hex(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
