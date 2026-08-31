// Package pf2 reads GRUB's bitmap font format.
//
// lint needs the name baked into a .pf2 -- GRUB matches on that, never on the
// filename -- and preview needs the glyphs, so its PNG shows the same aliased
// bitmap text the boot menu draws.
//
// Format (GRUB's docs/font_format.txt): a FILE header, then four-character
// sections with 32-bit big-endian lengths. CHIX indexes code point -> offset
// into DATA, where each glyph is a small header plus a 1-bit-per-pixel bitmap
// packed MSB first with no row padding.
package pf2

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"os"

	"github.com/sarbojitrana/grub-themes/internal/paint"
)

// Glyph is one character bitmap.
type Glyph struct {
	Width, Height int
	XOffset       int // from the pen position
	YOffset       int // from the baseline to the bottom of the bitmap
	DeviceWidth   int // how far to advance the pen
	bits          []byte
}

// On reports whether the pixel at x,y of the glyph bitmap is set.
func (g *Glyph) On(x, y int) bool {
	i := y*g.Width + x
	if i < 0 || i/8 >= len(g.bits) {
		return false
	}
	return g.bits[i/8]&(0x80>>(uint(i)%8)) != 0
}

// Font is a parsed .pf2.
type Font struct {
	Name      string
	Family    string
	PointSize int
	MaxWidth  int
	MaxHeight int
	Ascent    int
	Descent   int

	glyphs map[rune]*Glyph
}

// LineHeight is the natural distance between baselines.
func (f *Font) LineHeight() int { return f.Ascent + f.Descent }

// Glyph returns the bitmap for r, falling back to '?' and then to any glyph.
func (f *Font) Glyph(r rune) (*Glyph, bool) {
	if g, ok := f.glyphs[r]; ok {
		return g, true
	}
	if g, ok := f.glyphs['?']; ok {
		return g, false
	}
	return nil, false
}

// Measure returns the pen advance for s.
func (f *Font) Measure(s string) int {
	w := 0
	for _, r := range s {
		if g, _ := f.Glyph(r); g != nil {
			w += g.DeviceWidth
		}
	}
	return w
}

// Draw renders s with the pen at x and the baseline at y. Not anti-aliased:
// GRUB's fonts are 1 bit per pixel, and this is what the boot menu looks like.
func (f *Font) Draw(c *paint.Canvas, s string, x, y int, col color.NRGBA) int {
	pen := x
	for _, r := range s {
		g, _ := f.Glyph(r)
		if g == nil {
			continue
		}
		top := y - g.YOffset - g.Height
		left := pen + g.XOffset
		for gy := 0; gy < g.Height; gy++ {
			for gx := 0; gx < g.Width; gx++ {
				if g.On(gx, gy) {
					c.Blend(left+gx, top+gy, col, 1)
				}
			}
		}
		pen += g.DeviceWidth
	}
	return pen - x
}

// Parse reads a .pf2 file.
func Parse(path string) (*Font, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 12 || string(b[0:4]) != "FILE" || string(b[8:12]) != "PFF2" {
		return nil, fmt.Errorf("%s: not a PFF2 font", path)
	}

	f := &Font{glyphs: map[rune]*Glyph{}}
	var chix []byte
	var data []byte

	pos := 12
	for pos+8 <= len(b) {
		name := string(b[pos : pos+4])
		length := int(binary.BigEndian.Uint32(b[pos+4 : pos+8]))
		pos += 8
		if name == "DATA" || length == 0xFFFFFFFF || length < 0 {
			data = b
			break
		}
		if pos+length > len(b) {
			return nil, fmt.Errorf("%s: section %s runs past end of file", path, name)
		}
		body := b[pos : pos+length]
		switch name {
		case "NAME":
			f.Name = trimZero(string(body))
		case "FAMI":
			f.Family = trimZero(string(body))
		case "PTSZ":
			f.PointSize = int(be16(body))
		case "MAXW":
			f.MaxWidth = int(be16(body))
		case "MAXH":
			f.MaxHeight = int(be16(body))
		case "ASCE":
			f.Ascent = int(be16(body))
		case "DESC":
			f.Descent = int(be16(body))
		case "CHIX":
			chix = body
		}
		pos += length
	}
	if chix == nil || data == nil {
		return nil, fmt.Errorf("%s: missing CHIX or DATA section", path)
	}

	// 9 bytes each: code point, storage flags, offset.
	for i := 0; i+9 <= len(chix); i += 9 {
		code := rune(binary.BigEndian.Uint32(chix[i : i+4]))
		off := int(binary.BigEndian.Uint32(chix[i+5 : i+9]))
		g, err := readGlyph(data, off)
		if err != nil {
			continue // a single unreadable glyph is not worth failing over
		}
		f.glyphs[code] = g
	}
	if len(f.glyphs) == 0 {
		return nil, fmt.Errorf("%s: no glyphs", path)
	}
	if f.Ascent == 0 {
		f.Ascent = f.MaxHeight
	}
	return f, nil
}

func readGlyph(b []byte, off int) (*Glyph, error) {
	if off < 0 || off+10 > len(b) {
		return nil, fmt.Errorf("glyph offset out of range")
	}
	g := &Glyph{
		Width:       int(be16(b[off:])),
		Height:      int(be16(b[off+2:])),
		XOffset:     int(int16(be16(b[off+4:]))),
		YOffset:     int(int16(be16(b[off+6:]))),
		DeviceWidth: int(int16(be16(b[off+8:]))),
	}
	n := (g.Width*g.Height + 7) / 8
	if off+10+n > len(b) {
		return nil, fmt.Errorf("glyph bitmap out of range")
	}
	g.bits = b[off+10 : off+10+n]
	return g, nil
}

// Name is the string theme.txt has to reference.
func Name(path string) (string, error) {
	f, err := Parse(path)
	if err != nil {
		return "", err
	}
	return f.Name, nil
}

func be16(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func trimZero(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i]
		}
	}
	return s
}
