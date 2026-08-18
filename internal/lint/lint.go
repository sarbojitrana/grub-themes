// Package lint validates a theme without needing GRUB installed.
//
// Every check here exists because the corresponding mistake is invisible at
// build time and only shows up as a broken boot menu. GRUB has essentially no
// error reporting: it drops what it cannot read and carries on.
package lint

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sarbojitrana/grub-themes/internal/theme"
)

// Severity distinguishes "this will not render" from "this looks wrong".
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warn"
}

// Finding is a single problem found in a theme.
type Finding struct {
	Severity Severity
	File     string
	Message  string
	Hint     string
}

// Result collects findings for one theme.
type Result struct {
	Theme    theme.Theme
	Findings []Finding
}

func (r *Result) add(s Severity, file, msg, hint string) {
	r.Findings = append(r.Findings, Finding{s, file, msg, hint})
}

// HasErrors reports whether anything would stop the theme rendering.
func (r Result) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Check runs every validation against a theme.
func Check(t theme.Theme) Result {
	r := Result{Theme: t}
	checkManifest(&r)
	entry := t.EntryPath()
	src, err := os.ReadFile(entry)
	if err != nil {
		r.add(Error, filepath.Base(entry), "cannot read the theme entry file",
			"files.entry in theme.toml must point at your GRUB theme definition")
		return r
	}
	checkReferences(&r, string(src))
	checkPNGs(&r)
	checkFonts(&r, string(src))
	return r
}

func checkManifest(r *Result) {
	m := r.Theme.Manifest
	req := map[string]string{
		"theme.name":    m.Theme.Name,
		"theme.version": m.Theme.Version,
		"theme.license": m.Theme.License,
		"author.name":   m.Author.Name,
	}
	for field, val := range req {
		if strings.TrimSpace(val) == "" {
			r.add(Error, "theme.toml", "missing required field "+field, "")
		}
	}
	if p := r.Theme.PreviewPath(); p != "" {
		if _, err := os.Stat(p); err != nil {
			r.add(Warning, "theme.toml",
				"preview image "+m.Files.Preview+" does not exist",
				"a preview is what people see when browsing themes")
		}
	} else {
		r.add(Warning, "theme.toml", "no files.preview set",
			"add a preview image so the theme browser can show it")
	}
}

var (
	reQuoted  = regexp.MustCompile(`"([^"]+)"`)
	reFontRef = regexp.MustCompile(`(?m)^\s*(?:terminal-font|item_font|selected_item_font|font)\s*[:=]\s*"([^"]+)"`)
	reImgRef  = regexp.MustCompile(`(?m)^\s*desktop-image\s*:\s*"([^"]+)"`)
	rePixmap  = regexp.MustCompile(`(?m)^\s*(?:\w+_style|terminal-box)\s*[:=]\s*"([^"]+\*\.png)"`)
)

// checkReferences verifies every file named in theme.txt actually exists.
func checkReferences(r *Result, src string) {
	dir := r.Theme.Dir

	for _, m := range reImgRef.FindAllStringSubmatch(src, -1) {
		if _, err := os.Stat(filepath.Join(dir, m[1])); err != nil {
			r.add(Error, "theme.txt", "desktop-image "+m[1]+" does not exist", "")
		}
	}

	// "select_*.png" is a slice set: GRUB looks for _c, _w, _e (and the
	// corner/edge variants for boxes). Missing slices are a silent failure.
	for _, m := range rePixmap.FindAllStringSubmatch(src, -1) {
		pattern := m[1]
		base := strings.TrimSuffix(pattern, "_*.png")
		matches, _ := filepath.Glob(filepath.Join(dir, base+"_*.png"))
		if len(matches) == 0 {
			r.add(Error, "theme.txt",
				"no slices found for "+pattern,
				"GRUB expects files like "+base+"_c.png, "+base+"_w.png, "+base+"_e.png")
		}
	}
}

// pngHeader reads width, height, bit depth and colour type from IHDR.
func pngHeader(path string) (w, h uint32, depth, colourType byte, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	i := bytes.Index(b, []byte("IHDR"))
	if i < 0 || len(b) < i+14 {
		err = fmt.Errorf("no IHDR chunk")
		return
	}
	w = binary.BigEndian.Uint32(b[i+4 : i+8])
	h = binary.BigEndian.Uint32(b[i+8 : i+12])
	depth = b[i+12]
	colourType = b[i+13]
	return
}

// checkPNGs is the important one.
//
// GRUB decodes ONLY colour-type 6 (truecolour+alpha) at bit depth 8, and it
// fails silently. ImageMagick will happily optimise a flat-coloured rectangle
// down to a 1-bit palette PNG, which GRUB drops entirely -- the selected menu
// entry then looks blanked out rather than highlighted, and a themed terminal
// box turns into an opaque black slab during boot.
//
// Note you cannot check this with `magick identify -format '%[type]'`: it
// reports "PaletteAlpha" for a perfectly valid colour-type 6 file, because it
// describes the content rather than the encoding. Read IHDR, as here.
func checkPNGs(r *Result) {
	pngs, _ := filepath.Glob(filepath.Join(r.Theme.Dir, "*.png"))
	for _, p := range pngs {
		_, _, depth, ct, err := pngHeader(p)
		name := filepath.Base(p)
		if err != nil {
			r.add(Error, name, "not a readable PNG: "+err.Error(), "")
			continue
		}
		if ct != 6 || depth != 8 {
			r.add(Error, name,
				fmt.Sprintf("PNG is colour-type %d depth %d; GRUB needs colour-type 6 depth 8", ct, depth),
				"regenerate with: magick ... -define png:color-type=6 -define png:bit-depth=8 PNG32:out.png")
		}
	}
}

// checkFonts verifies each font named in theme.txt matches a name baked into
// one of the .pf2 files. GRUB matches fonts by that internal name, never by
// filename, so a renamed file or a resized font silently renders nothing.
func checkFonts(r *Result, src string) {
	pf2s, _ := filepath.Glob(filepath.Join(r.Theme.Dir, "*.pf2"))
	names := map[string]string{} // font name -> file it came from
	for _, p := range pf2s {
		for _, n := range pf2Names(p) {
			names[n] = filepath.Base(p)
		}
	}

	seen := map[string]bool{}
	for _, m := range reFontRef.FindAllStringSubmatch(src, -1) {
		want := m[1]
		if seen[want] {
			continue
		}
		seen[want] = true
		if _, ok := names[want]; !ok {
			r.add(Error, "theme.txt",
				"font \""+want+"\" is not baked into any .pf2 in this theme",
				"run the theme's build-fonts script; it prints the names to use")
		}
	}
	if len(pf2s) == 0 {
		r.add(Warning, "theme.txt", "no .pf2 fonts shipped with this theme",
			"GRUB will fall back to its built-in font")
	}
}

// pf2Names extracts printable name strings from a .pf2 font.
func pf2Names(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// The NAME section sits near the start; scanning the first few KB is
	// enough and avoids reading a megabyte of glyph data.
	buf := make([]byte, 4096)
	n, _ := bufio.NewReader(f).Read(buf)
	buf = buf[:n]

	var out []string
	var cur []byte
	for _, c := range buf {
		if c >= 0x20 && c < 0x7f {
			cur = append(cur, c)
			continue
		}
		if len(cur) >= 6 {
			out = append(out, string(cur))
		}
		cur = nil
	}
	if len(cur) >= 6 {
		out = append(out, string(cur))
	}
	return out
}
