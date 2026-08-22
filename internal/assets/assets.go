// Package assets generates a theme's pixmaps from the declarative [assets]
// section of theme.toml.
//
// This exists so that adding a theme needs no ImageMagick and no shell. The
// one rule a theme author must never get wrong -- every PNG has to be
// colour-type 6 at bit depth 8 or GRUB silently renders nothing -- is enforced
// here, in one place, by paint.Save.
//
// The slice sets below deliberately mirror the JARVIS theme, which is the
// layout known to render correctly on real hardware:
//
//	select_{w,c,e}.png            horizontal 3-slice highlight
//	terminal_box_{c,n,s,w,e,nw,ne,sw,se}.png   full 9-slice box
//	progress_bar_c.png, progress_highlight_c.png
package assets

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"

	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// Defaults, chosen to match themes/jarvis so that regenerating it produces the
// same pixmaps the hand-written ImageMagick script used to.
const (
	defSelHeight    = 44
	defSelWidth     = 240
	defSelRadius    = 10
	defCentreWidth  = 32
	defProgressH    = 8
	defUnderlineTop = 4
)

// Report is what one build produced.
type Report struct {
	Written []string     // pixmap paths
	Fonts   []FontResult // .pf2 files, with the name GRUB matches on
	Notes   []string     // why something was skipped, if it was
}

// Build regenerates every asset declared by the theme.
func Build(t theme.Theme) (Report, error) {
	var rep Report
	a := t.Manifest.Assets
	if a == nil {
		return rep, fmt.Errorf("%s has no [assets] section in theme.toml; nothing to build",
			t.Manifest.Theme.ID)
	}

	sel, err := buildSelection(t, a)
	if err != nil {
		return rep, err
	}
	rep.Written = append(rep.Written, sel...)

	box, err := buildTerminalBox(t, a)
	if err != nil {
		return rep, err
	}
	rep.Written = append(rep.Written, box...)

	prog, err := buildProgress(t, a)
	if err != nil {
		return rep, err
	}
	rep.Written = append(rep.Written, prog...)

	bg, bgNote, err := buildBackground(t, a.Background)
	if err != nil {
		return rep, err
	}
	if bg != "" {
		rep.Written = append(rep.Written, bg)
	}

	fonts, fontNote, err := buildFonts(t, a.Fonts)
	if err != nil {
		return rep, err
	}
	rep.Fonts = fonts
	for _, n := range []string{bgNote, fontNote} {
		if n != "" {
			rep.Notes = append(rep.Notes, n)
		}
	}
	return rep, nil
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// buildSelection draws the highlight as one long pill and cuts it into the
// three slices GRUB tiles: a left cap, a repeatable centre, a right cap.
func buildSelection(t theme.Theme, a *theme.Assets) ([]string, error) {
	s := a.Selection
	style := s.Style
	if style == "" {
		style = "pill"
	}
	if style == "none" {
		return nil, nil
	}

	h := intOr(s.Height, defSelHeight)
	w := intOr(s.Width, defSelWidth)
	radius := intOr(s.Radius, defSelRadius)
	switch style {
	case "bar":
		radius = intOr(s.Radius, 0)
	case "underline":
		radius = 0
	case "pill":
	default:
		return nil, fmt.Errorf("unknown selection style %q (want pill, bar, underline or none)", style)
	}

	fill, err := paint.Hex(s.Fill)
	if err != nil {
		return nil, fmt.Errorf("assets.selection.fill: %w", err)
	}
	shadow, err := paint.Hex(s.Shadow)
	if err != nil {
		return nil, fmt.Errorf("assets.selection.shadow: %w", err)
	}
	dx, dy := intOr(s.ShadowDX, 0), intOr(s.ShadowDY, 0)
	if shadow.A == 0 {
		dx, dy = 0, 0
	}

	// The shadow has to fit inside the slice, so the body is inset by the
	// offset rather than the canvas being grown -- item_height in theme.txt
	// stays honest.
	bodyW, bodyH := float64(w-abs(dx)), float64(h-abs(dy))
	ox, oy := 0.0, 0.0
	if dx < 0 {
		ox = float64(-dx)
	}
	if dy < 0 {
		oy = float64(-dy)
	}

	c := paint.New(w, h)
	draw := func(x, y, ww, hh float64, col color.NRGBA) {
		if style == "underline" {
			th := float64(intOr(s.Thickness, defUnderlineTop))
			c.FillRoundRect(x, y+hh-th, ww, th, 0, col)
			return
		}
		c.FillRoundRect(x, y, ww, hh, float64(radius), col)
	}
	if shadow.A > 0 {
		draw(ox+float64(dx), oy+float64(dy), bodyW, bodyH, shadow)
	}
	draw(ox, oy, bodyW, bodyH, fill)

	capW := radius * 2
	if capW < 6 {
		capW = 6
	}
	if extra := abs(dx) + 4; capW < extra {
		capW = extra
	}
	if capW > w/2-2 {
		capW = w/2 - 2
	}

	dir := t.Dir
	mid := (w - defCentreWidth) / 2
	out := []struct {
		name string
		rect image.Rectangle
	}{
		{"select_w.png", image.Rect(0, 0, capW, h)},
		{"select_c.png", image.Rect(mid, 0, mid+defCentreWidth, h)},
		{"select_e.png", image.Rect(w-capW, 0, w, h)},
	}
	var written []string
	for _, o := range out {
		p := filepath.Join(dir, o.name)
		if err := paint.Save(p, paint.Crop(c, o.rect)); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	return written, nil
}

// buildTerminalBox writes the 9 slices GRUB wants for terminal-box.
//
// Transparent by default, and that is not laziness: GRUB draws this box
// whenever a menu entry prints output, which happens the moment you press
// Enter. Any visible fill therefore sits on top of the theme for as long as
// the kernel takes to load.
func buildTerminalBox(t theme.Theme, a *theme.Assets) ([]string, error) {
	fill, err := paint.Hex(a.TerminalBox.Fill)
	if err != nil {
		return nil, fmt.Errorf("assets.terminal_box.fill: %w", err)
	}
	sizes := map[string][2]int{
		"c":  {32, 32},
		"n":  {32, 2},
		"s":  {32, 2},
		"w":  {2, 32},
		"e":  {2, 32},
		"nw": {2, 2},
		"ne": {2, 2},
		"sw": {2, 2},
		"se": {2, 2},
	}
	var written []string
	for _, slice := range []string{"c", "n", "s", "w", "e", "nw", "ne", "sw", "se"} {
		sz := sizes[slice]
		c := paint.New(sz[0], sz[1])
		if fill.A > 0 {
			c.Fill(fill)
		}
		p := filepath.Join(t.Dir, "terminal_box_"+slice+".png")
		if err := paint.Save(p, c); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	return written, nil
}

func buildProgress(t theme.Theme, a *theme.Assets) ([]string, error) {
	if a.Progress.Track == "" && a.Progress.Fill == "" {
		return nil, nil
	}
	track, err := paint.Hex(a.Progress.Track)
	if err != nil {
		return nil, fmt.Errorf("assets.progress.track: %w", err)
	}
	fill, err := paint.Hex(a.Progress.Fill)
	if err != nil {
		return nil, fmt.Errorf("assets.progress.fill: %w", err)
	}
	h := intOr(a.Progress.Height, defProgressH)

	var written []string
	for _, o := range []struct {
		name string
		col  color.NRGBA
	}{
		{"progress_bar_c.png", track},
		{"progress_highlight_c.png", fill},
	} {
		c := paint.New(32, h)
		c.Fill(o.col)
		p := filepath.Join(t.Dir, o.name)
		if err := paint.Save(p, c); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	return written, nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
