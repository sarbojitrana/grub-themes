package paint

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The invariant the whole package exists for: GRUB reads colour-type 6 at
// depth 8 and nothing else, and Go's encoder drops alpha when opaque.
func TestSaveAlwaysWritesColourType6(t *testing.T) {
	for _, tc := range []struct {
		name string
		col  color.NRGBA
	}{
		{"opaque", color.NRGBA{R: 0, G: 217, B: 255, A: 255}},
		{"translucent", color.NRGBA{R: 18, G: 32, B: 42, A: 217}},
		{"transparent", color.NRGBA{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(16, 8)
			c.Fill(tc.col)
			p := filepath.Join(t.TempDir(), "out.png")
			if err := Save(p, c); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			depth, ct, err := ihdr(b)
			if err != nil {
				t.Fatal(err)
			}
			if ct != 6 || depth != 8 {
				t.Fatalf("colour-type %d depth %d, want 6/8", ct, depth)
			}
			img, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			if got := color.NRGBAModel.Convert(img.At(3, 3)).(color.NRGBA); got != tc.col {
				t.Errorf("round trip changed the pixel: %v -> %v", tc.col, got)
			}
		})
	}
}

func TestHex(t *testing.T) {
	cases := map[string]color.NRGBA{
		"#00d9ff":     {R: 0, G: 0xd9, B: 0xff, A: 255},
		"#fff":        {R: 255, G: 255, B: 255, A: 255},
		"#12202ad9":   {R: 0x12, G: 0x20, B: 0x2a, A: 0xd9},
		"transparent": {},
		"":            {},
		"00d9ff":      {R: 0, G: 0xd9, B: 0xff, A: 255},
	}
	for in, want := range cases {
		got, err := Hex(in)
		if err != nil {
			t.Errorf("Hex(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Hex(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := Hex("#xyz123"); err == nil {
		t.Error("expected an error for a bad colour")
	}
}

func TestContrast(t *testing.T) {
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{A: 255}
	if got := Contrast(white, black); math.Abs(got-21) > 0.01 {
		t.Errorf("black on white = %.2f, want 21", got)
	}
	if got := Contrast(white, white); math.Abs(got-1) > 0.001 {
		t.Errorf("white on white = %.2f, want 1", got)
	}
}

func TestFillRoundRectIsAntiAliased(t *testing.T) {
	c := New(40, 20)
	c.FillRoundRect(0, 0, 40, 20, 8, color.NRGBA{R: 255, A: 255})
	if a := c.NRGBAAt(20, 10).A; a != 255 {
		t.Errorf("centre alpha = %d, want opaque", a)
	}
	if a := c.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("corner alpha = %d, want transparent", a)
	}
	edge := c.NRGBAAt(2, 1).A
	if edge == 0 || edge == 255 {
		t.Errorf("corner edge alpha = %d, want partial coverage", edge)
	}
}
