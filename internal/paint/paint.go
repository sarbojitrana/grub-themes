// Package paint is a very small raster toolkit: enough to draw the pixmaps a
// GRUB theme needs, composite a preview, and write PNGs that GRUB can actually
// decode.
//
// The last part is the whole reason this package exists. GRUB reads ONLY
// colour-type 6 (truecolour+alpha) at bit depth 8 and fails silently on
// anything else, so Save always encodes from an *image.NRGBA -- which Go's PNG
// encoder writes as colour-type 6 -- and then reads the IHDR back to prove it.
package paint

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Canvas is an RGBA image with straight (non-premultiplied) alpha.
type Canvas struct {
	*image.NRGBA
}

// New returns a fully transparent canvas.
func New(w, h int) *Canvas {
	return &Canvas{image.NewNRGBA(image.Rect(0, 0, w, h))}
}

// W and H are the canvas dimensions.
func (c *Canvas) W() int { return c.Bounds().Dx() }
func (c *Canvas) H() int { return c.Bounds().Dy() }

// Fill paints the whole canvas.
func (c *Canvas) Fill(col color.NRGBA) {
	for y := 0; y < c.H(); y++ {
		for x := 0; x < c.W(); x++ {
			c.SetNRGBA(x, y, col)
		}
	}
}

// Blend composites col over the pixel at x,y with coverage a in [0,1].
func (c *Canvas) Blend(x, y int, col color.NRGBA, a float64) {
	if a <= 0 || x < 0 || y < 0 || x >= c.W() || y >= c.H() {
		return
	}
	if a > 1 {
		a = 1
	}
	sa := float64(col.A) / 255 * a
	if sa <= 0 {
		return
	}
	dst := c.NRGBAAt(x, y)
	da := float64(dst.A) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		c.SetNRGBA(x, y, color.NRGBA{})
		return
	}
	mix := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / oa
		return clamp8(v)
	}
	c.SetNRGBA(x, y, color.NRGBA{
		R: mix(col.R, dst.R),
		G: mix(col.G, dst.G),
		B: mix(col.B, dst.B),
		A: clamp8(oa * 255),
	})
}

// FillRect paints an axis-aligned rectangle.
func (c *Canvas) FillRect(x, y, w, h int, col color.NRGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.Blend(i, j, col, 1)
		}
	}
}

// FillRoundRect paints a rounded rectangle, anti-aliased by 4x4 supersampling.
// r is the corner radius; r <= 0 gives square corners.
func (c *Canvas) FillRoundRect(x, y, w, h, r float64, col color.NRGBA) {
	if r < 0 {
		r = 0
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	const ss = 4
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := int(math.Ceil(x+w)), int(math.Ceil(y+h))
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			hits := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(px) + (float64(sx)+0.5)/ss
					fy := float64(py) + (float64(sy)+0.5)/ss
					if insideRoundRect(fx, fy, x, y, w, h, r) {
						hits++
					}
				}
			}
			if hits > 0 {
				c.Blend(px, py, col, float64(hits)/(ss*ss))
			}
		}
	}
}

func insideRoundRect(px, py, x, y, w, h, r float64) bool {
	if px < x || py < y || px > x+w || py > y+h {
		return false
	}
	// Distance to the nearest corner centre, only inside the corner boxes.
	cx, cy := px, py
	switch {
	case px < x+r:
		cx = x + r
	case px > x+w-r:
		cx = x + w - r
	default:
		return true
	}
	switch {
	case py < y+r:
		cy = y + r
	case py > y+h-r:
		cy = y + h - r
	default:
		return true
	}
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

// DrawImage composites src onto the canvas with its top-left at x,y.
func (c *Canvas) DrawImage(src image.Image, x, y int) {
	b := src.Bounds()
	for j := b.Min.Y; j < b.Max.Y; j++ {
		for i := b.Min.X; i < b.Max.X; i++ {
			r, g, bb, a := src.At(i, j).RGBA()
			if a == 0 {
				continue
			}
			// Un-premultiply back to straight alpha.
			af := float64(a) / 65535
			col := color.NRGBA{
				R: clamp8(float64(r) / 257 / af),
				G: clamp8(float64(g) / 257 / af),
				B: clamp8(float64(bb) / 257 / af),
				A: clamp8(af * 255),
			}
			c.Blend(x+i-b.Min.X, y+j-b.Min.Y, col, 1)
		}
	}
}

// TileX repeats src horizontally to fill w pixels starting at x,y.
func (c *Canvas) TileX(src image.Image, x, y, w int) {
	sw := src.Bounds().Dx()
	if sw <= 0 {
		return
	}
	for dx := 0; dx < w; dx += sw {
		n := sw
		if dx+n > w {
			n = w - dx
		}
		sub := src.(interface {
			SubImage(image.Rectangle) image.Image
		}).SubImage(image.Rect(
			src.Bounds().Min.X, src.Bounds().Min.Y,
			src.Bounds().Min.X+n, src.Bounds().Max.Y))
		c.DrawImage(sub, x+dx, y)
	}
}

// Scale resizes src to w x h with bilinear filtering.
func Scale(src image.Image, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return out
	}
	for y := 0; y < h; y++ {
		fy := (float64(y) + 0.5) * float64(sh) / float64(h)
		y0 := int(fy - 0.5)
		ty := fy - 0.5 - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x) + 0.5) * float64(sw) / float64(w)
			x0 := int(fx - 0.5)
			tx := fx - 0.5 - float64(x0)
			var rr, gg, bb, aa float64
			for j := 0; j < 2; j++ {
				for i := 0; i < 2; i++ {
					wx := tx
					if i == 0 {
						wx = 1 - tx
					}
					wy := ty
					if j == 0 {
						wy = 1 - ty
					}
					r, g, b2, a := src.At(
						b.Min.X+clampInt(x0+i, 0, sw-1),
						b.Min.Y+clampInt(y0+j, 0, sh-1)).RGBA()
					wgt := wx * wy
					rr += float64(r) * wgt
					gg += float64(g) * wgt
					bb += float64(b2) * wgt
					aa += float64(a) * wgt
				}
			}
			if aa == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: clamp8(rr / aa * 255),
				G: clamp8(gg / aa * 255),
				B: clamp8(bb / aa * 255),
				A: clamp8(aa / 65535 * 255),
			})
		}
	}
	return out
}

// Crop returns a copy of the given rectangle of src.
func Crop(src image.Image, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// Hex parses "#rrggbb", "#rgb", "#rrggbbaa" or the word "transparent".
func Hex(s string) (color.NRGBA, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "none", "transparent":
		return color.NRGBA{}, nil
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 && len(s) != 8 {
		return color.NRGBA{}, fmt.Errorf("bad colour %q: want #rgb, #rrggbb or #rrggbbaa", s)
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("bad colour %q: %w", s, err)
	}
	if len(s) == 6 {
		return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
	}
	return color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)}, nil
}

// Luminance is the WCAG relative luminance of a colour.
func Luminance(c color.NRGBA) float64 {
	f := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

// Contrast is the WCAG contrast ratio between two colours, 1..21.
func Contrast(a, b color.NRGBA) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Save writes img as a PNG that GRUB can decode: colour-type 6, bit depth 8,
// non-interlaced. It converts to NRGBA first (Go's encoder picks colour-type 6
// for that) and then verifies the IHDR it just wrote, because getting this
// wrong is invisible until the boot menu renders nothing.
func Save(path string, img image.Image) error {
	n, ok := img.(*image.NRGBA)
	if !ok {
		b := img.Bounds()
		n = image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				n.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	var buf bytes.Buffer
	if err := encodePNG(&buf, n); err != nil {
		return err
	}
	depth, ct, err := ihdr(buf.Bytes())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if ct != 6 || depth != 8 {
		return fmt.Errorf("%s: encoded as colour-type %d depth %d, GRUB needs 6/8", path, ct, depth)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Load decodes a PNG from disk.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func ihdr(b []byte) (depth, colourType byte, err error) {
	i := bytes.Index(b, []byte("IHDR"))
	if i < 0 || len(b) < i+14 {
		return 0, 0, fmt.Errorf("no IHDR chunk")
	}
	_ = binary.BigEndian.Uint32(b[i+4 : i+8])
	return b[i+12], b[i+13], nil
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
