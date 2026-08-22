package paint

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"io"
)

// encodePNG writes img as colour-type 6 (truecolour+alpha), bit depth 8,
// non-interlaced.
//
// Go's image/png encoder cannot be used for this. It inspects the image and
// writes colour-type 2 whenever every pixel is opaque -- a sensible size
// optimisation everywhere except here, because GRUB decodes colour-type 6 and
// nothing else, silently. A fully opaque background would come back as a blank
// screen with no error anywhere, so the encoding is pinned by hand instead.
func encodePNG(w io.Writer, img *image.NRGBA) error {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()

	if _, err := w.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); err != nil {
		return err
	}

	var ihdr bytes.Buffer
	binary.Write(&ihdr, binary.BigEndian, uint32(width))
	binary.Write(&ihdr, binary.BigEndian, uint32(height))
	ihdr.Write([]byte{
		8, // bit depth
		6, // colour type: truecolour + alpha
		0, // deflate
		0, // adaptive filtering
		0, // no interlace
	})
	if err := chunk(w, "IHDR", ihdr.Bytes()); err != nil {
		return err
	}

	const bpp = 4
	stride := width * bpp
	raw := make([]byte, stride)
	prev := make([]byte, stride)
	cand := make([][]byte, 5)
	for i := range cand {
		cand[i] = make([]byte, stride)
	}

	var idat bytes.Buffer
	z, err := zlib.NewWriterLevel(&idat, zlib.BestCompression)
	if err != nil {
		return err
	}
	for y := 0; y < height; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+stride]
		copy(raw, row)
		ft, filtered := filterRow(raw, prev, cand, bpp)
		if _, err := z.Write([]byte{ft}); err != nil {
			return err
		}
		if _, err := z.Write(filtered); err != nil {
			return err
		}
		copy(prev, raw)
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := chunk(w, "IDAT", idat.Bytes()); err != nil {
		return err
	}
	return chunk(w, "IEND", nil)
}

// filterRow applies each PNG filter and keeps the one with the smallest sum of
// absolute differences, the heuristic libpng uses. It matters: these files are
// committed to the repository, and a 1920x1080 background unfiltered is several
// times larger.
func filterRow(raw, prev []byte, cand [][]byte, bpp int) (byte, []byte) {
	n := len(raw)
	copy(cand[0], raw)
	for i := 0; i < n; i++ {
		var a, b, c byte
		if i >= bpp {
			a = raw[i-bpp]
			c = prev[i-bpp]
		}
		b = prev[i]
		cand[1][i] = raw[i] - a
		cand[2][i] = raw[i] - b
		cand[3][i] = raw[i] - byte((int(a)+int(b))/2)
		cand[4][i] = raw[i] - paeth(a, b, c)
	}
	best, bestScore := byte(0), ^uint64(0)
	for f := 0; f < 5; f++ {
		var score uint64
		for _, v := range cand[f] {
			if v < 128 {
				score += uint64(v)
			} else {
				score += uint64(256 - int(v))
			}
		}
		if score < bestScore {
			best, bestScore = byte(f), score
		}
	}
	return best, cand[best]
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	default:
		return c
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func chunk(w io.Writer, typ string, data []byte) error {
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	crc := crc32.NewIEEE()
	if _, err := io.WriteString(io.MultiWriter(w, crc), typ); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := io.MultiWriter(w, crc).Write(data); err != nil {
			return err
		}
	}
	return binary.Write(w, binary.BigEndian, crc.Sum32())
}
