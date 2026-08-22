package preview

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/pf2"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// DefaultEntries is a realistic boot menu: a normal entry, another OS, two
// long entries that show where text overflows, and a firmware entry.
var DefaultEntries = []string{
	"Arch Linux",
	"Windows Boot Manager",
	"Arch Linux :: Recovery (fallback initramfs)",
	"Arch Linux :: Emergency shell (rescue target)",
	"UEFI Firmware Settings",
}

// Options controls what the rendered menu shows.
type Options struct {
	Width, Height int
	Entries       []string
	Selected      int
	Timeout       int     // substituted into "%d" in timeout labels
	Progress      float64 // 0..1, how far the countdown bar has run
}

func (o *Options) applyDefaults(t theme.Theme) {
	if len(o.Entries) == 0 {
		o.Entries = DefaultEntries
	}
	if o.Timeout == 0 {
		o.Timeout = 3
	}
	if o.Progress == 0 {
		o.Progress = 0.62
	}
	if o.Width == 0 || o.Height == 0 {
		o.Width, o.Height = 1920, 1080
		if d := t.Manifest.Display.DesignedFor; d != "" {
			if w, h, ok := parseSize(d); ok {
				o.Width, o.Height = w, h
			}
		}
	}
}

func parseSize(s string) (int, int, bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(s)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// renderer holds everything one render needs.
type renderer struct {
	t     theme.Theme
	doc   document
	opt   Options
	c     *paint.Canvas
	fonts map[string]*pf2.Font
}

// Render draws the theme's menu into a canvas.
func Render(t theme.Theme, opt Options) (*paint.Canvas, error) {
	opt.applyDefaults(t)

	src, err := os.ReadFile(t.EntryPath())
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", t.EntryPath(), err)
	}
	r := &renderer{
		t:     t,
		doc:   parse(string(src)),
		opt:   opt,
		c:     paint.New(opt.Width, opt.Height),
		fonts: map[string]*pf2.Font{},
	}

	// GRUB matches fonts by the name baked into the .pf2, never by filename.
	paths, _ := filepath.Glob(filepath.Join(t.Dir, "*.pf2"))
	for _, p := range paths {
		if f, err := pf2.Parse(p); err == nil {
			r.fonts[f.Name] = f
		}
	}

	r.drawBackground()
	for _, comp := range r.doc.Components {
		switch comp.Name {
		case "boot_menu":
			r.drawBootMenu(comp)
		case "progress_bar":
			r.drawProgressBar(comp)
		case "label":
			r.drawLabel(comp)
		}
	}
	r.drawTitle()
	return r.c, nil
}

// Write renders the theme and saves it as a GRUB-safe PNG.
func Write(t theme.Theme, path string, opt Options) error {
	c, err := Render(t, opt)
	if err != nil {
		return err
	}
	return paint.Save(path, c)
}

func (r *renderer) hex(s string, def color.NRGBA) color.NRGBA {
	if s == "" {
		return def
	}
	c, err := paint.Hex(s)
	if err != nil {
		return def
	}
	return c
}

func (r *renderer) font(name string) *pf2.Font {
	if f, ok := r.fonts[name]; ok {
		return f
	}
	// Fall back to the largest font shipped, so a theme with a typo in a font
	// name still previews -- lint is what reports the typo.
	var best *pf2.Font
	for _, f := range r.fonts {
		if best == nil || f.MaxHeight > best.MaxHeight {
			best = f
		}
	}
	return best
}

func (r *renderer) drawBackground() {
	r.c.Fill(r.hex(r.doc.global("desktop-color"), color.NRGBA{R: 8, G: 8, B: 8, A: 255}))
	img := r.doc.global("desktop-image")
	if img == "" {
		return
	}
	bg, err := paint.Load(filepath.Join(r.t.Dir, img))
	if err != nil {
		return
	}
	// GRUB stretches the desktop image to the screen by default.
	r.c.DrawImage(paint.Scale(bg, r.opt.Width, r.opt.Height), 0, 0)
}

func (r *renderer) drawTitle() {
	text := r.doc.global("title-text")
	if text == "" {
		return
	}
	f := r.font(r.doc.global("title-font"))
	if f == nil {
		return
	}
	col := r.hex(r.doc.global("title-color"), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	x := (r.opt.Width - f.Measure(text)) / 2
	f.Draw(r.c, text, x, 40+f.Ascent, col)
}

func (r *renderer) drawBootMenu(c component) {
	left := dim(c.getOr("left", "0"), r.opt.Width)
	top := dim(c.getOr("top", "0"), r.opt.Height)
	width := dim(c.getOr("width", "100%"), r.opt.Width)
	height := dim(c.getOr("height", "100%"), r.opt.Height)

	itemH := c.num("item_height", 36)
	spacing := c.num("item_spacing", 0)
	padding := c.num("item_padding", 0)
	iconW := c.num("icon_width", 0)
	iconSpace := c.num("item_icon_space", 0)

	itemFont := r.font(c.get("item_font"))
	selFont := r.font(c.getOr("selected_item_font", c.get("item_font")))
	itemCol := r.hex(c.get("item_color"), color.NRGBA{R: 220, G: 220, B: 220, A: 255})
	selCol := r.hex(c.getOr("selected_item_color", c.get("item_color")), itemCol)

	slices := r.loadSlices(c.get("selected_item_pixmap_style"))

	// GRUB draws the highlight box *around* the item content, so the west
	// slice sits to the left of where the text starts -- and unselected items
	// line up with the selected one because both are positioned from the
	// content edge, not the box edge.
	border := 0
	if slices != nil && slices.w != nil {
		border = slices.w.Bounds().Dx()
	}
	boxX := left + padding
	boxW := width - 2*padding
	textX := boxX + border + iconW + iconSpace

	for i, entry := range r.opt.Entries {
		y := top + padding + i*(itemH+spacing)
		if y+itemH > top+height {
			break
		}
		f, col := itemFont, itemCol
		if i == r.opt.Selected {
			r.drawSlices(slices, boxX, y, boxW, itemH)
			f, col = selFont, selCol
		}
		if f == nil {
			continue
		}
		baseline := y + (itemH+f.Ascent-f.Descent)/2
		f.Draw(r.c, entry, textX, baseline, col)
	}
}

// sliceSet is the west/centre/east trio GRUB tiles for a horizontal box.
type sliceSet struct {
	w, c, e image.Image
}

func (r *renderer) loadSlices(pattern string) *sliceSet {
	if pattern == "" || !strings.Contains(pattern, "*") {
		return nil
	}
	base := strings.TrimSuffix(pattern, "_*.png")
	load := func(suffix string) image.Image {
		img, err := paint.Load(filepath.Join(r.t.Dir, base+"_"+suffix+".png"))
		if err != nil {
			return nil
		}
		return img
	}
	s := &sliceSet{w: load("w"), c: load("c"), e: load("e")}
	if s.c == nil {
		return nil
	}
	return s
}

// drawSlices reproduces GRUB's horizontal 3-slice box: the caps keep their
// width, the centre repeats across the gap, everything scaled to the item
// height.
func (r *renderer) drawSlices(s *sliceSet, x, y, w, h int) {
	if s == nil || w <= 0 || h <= 0 {
		return
	}
	scaleTo := func(img image.Image) *image.NRGBA {
		if img == nil {
			return nil
		}
		b := img.Bounds()
		if b.Dy() == h {
			return paint.Crop(img, b)
		}
		return paint.Scale(img, b.Dx(), h)
	}
	west, centre, east := scaleTo(s.w), scaleTo(s.c), scaleTo(s.e)

	lw, rw := 0, 0
	if west != nil {
		lw = west.Bounds().Dx()
	}
	if east != nil {
		rw = east.Bounds().Dx()
	}
	if lw+rw > w {
		lw, rw = w/2, w-w/2
	}
	if west != nil {
		r.c.DrawImage(paint.Crop(west, image.Rect(0, 0, lw, h)), x, y)
	}
	if centre != nil {
		r.c.TileX(centre, x+lw, y, w-lw-rw)
	}
	if east != nil {
		r.c.DrawImage(paint.Crop(east, image.Rect(east.Bounds().Dx()-rw, 0, east.Bounds().Dx(), h)), x+w-rw, y)
	}
}

func (r *renderer) drawProgressBar(c component) {
	left := dim(c.getOr("left", "0"), r.opt.Width)
	top := dim(c.getOr("top", "0"), r.opt.Height)
	width := dim(c.getOr("width", "100%"), r.opt.Width)
	height := dim(c.getOr("height", "10"), r.opt.Height)
	if width <= 0 || height <= 0 {
		return
	}

	filled := int(float64(width) * r.opt.Progress)

	if track := r.loadSlices(c.get("bar_style")); track != nil {
		r.drawSlices(track, left, top, width, height)
	} else {
		r.c.FillRect(left, top, width, height, r.hex(c.get("bg_color"), color.NRGBA{R: 40, G: 40, B: 40, A: 255}))
	}
	if hl := r.loadSlices(c.get("highlight_style")); hl != nil {
		r.drawSlices(hl, left, top, filled, height)
	} else {
		r.c.FillRect(left, top, filled, height, r.hex(c.get("fg_color"), color.NRGBA{R: 200, G: 200, B: 200, A: 255}))
	}
}

func (r *renderer) drawLabel(c component) {
	text := c.get("text")
	if text == "" {
		return
	}
	// GRUB substitutes the seconds remaining into a timeout label.
	if strings.Contains(text, "%d") {
		text = strings.ReplaceAll(text, "%d", strconv.Itoa(r.opt.Timeout))
	}
	f := r.font(c.get("font"))
	if f == nil {
		return
	}
	left := dim(c.getOr("left", "0"), r.opt.Width)
	top := dim(c.getOr("top", "0"), r.opt.Height)
	width := dim(c.getOr("width", "0"), r.opt.Width)
	col := r.hex(c.get("color"), color.NRGBA{R: 220, G: 220, B: 220, A: 255})

	x := left
	switch c.get("align") {
	case "center":
		x = left + (width-f.Measure(text))/2
	case "right":
		x = left + width - f.Measure(text)
	}
	f.Draw(r.c, text, x, top+f.Ascent, col)
}
