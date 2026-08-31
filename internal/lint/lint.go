// Package lint validates a theme without needing GRUB installed.
//
// Every check here is for a mistake GRUB reports as nothing at all: it
// drops what it cannot read and carries on booting.
package lint

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/pf2"
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
	checkAssets(&r, string(src))
	return r
}

func checkManifest(r *Result) {
	m := r.Theme.Manifest
	// The id is what the theme is installed as.
	if base := filepath.Base(r.Theme.Dir); m.Theme.ID != base {
		r.add(Warning, "theme.toml",
			fmt.Sprintf("theme.id is %q but the directory is %q", m.Theme.ID, base),
			"keep them the same; the id is what the theme is installed as")
	}
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

	reSelectedColor = regexp.MustCompile(`(?m)^\s*selected_item_color\s*=\s*"([^"]+)"`)
	reItemHeight    = regexp.MustCompile(`(?m)^\s*item_height\s*=\s*(\d+)`)
)

// checkReferences verifies every file named in theme.txt actually exists.
func checkReferences(r *Result, src string) {
	dir := r.Theme.Dir

	for _, m := range reImgRef.FindAllStringSubmatch(src, -1) {
		if _, err := os.Stat(filepath.Join(dir, m[1])); err != nil {
			r.add(Error, "theme.txt", "desktop-image "+m[1]+" does not exist", "")
		}
	}

	// A slice set: GRUB wants _c, _w, _e, and reports none of them missing.
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

// checkPNGs is the important one: GRUB decodes only colour-type 6 at depth 8
// and fails silently, so a palette PNG leaves the selected entry looking
// blanked out. `magick identify -format '%[type]'` cannot tell you this --
// it describes content, not encoding -- so read the IHDR.
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

// checkFonts matches theme.txt against the names baked into the .pf2 files.
// GRUB never matches on filename, and a miss renders nothing.
func checkFonts(r *Result, src string) {
	pf2s, _ := filepath.Glob(filepath.Join(r.Theme.Dir, "*.pf2"))
	names := map[string]string{} // font name -> file it came from
	for _, p := range pf2s {
		n, err := pf2.Name(p)
		if err != nil {
			r.add(Error, filepath.Base(p), "cannot read this font: "+err.Error(), "")
			continue
		}
		names[n] = filepath.Base(p)
	}

	seen := map[string]bool{}
	for _, m := range reFontRef.FindAllStringSubmatch(src, -1) {
		want := m[1]
		if seen[want] {
			continue
		}
		seen[want] = true
		if _, ok := names[want]; !ok {
			have := make([]string, 0, len(names))
			for n := range names {
				have = append(have, n)
			}
			sort.Strings(have)
			hint := "run `grub-themes build " + r.Theme.Manifest.Theme.ID + "`; it prints the names to use"
			if len(have) > 0 {
				hint += " (this theme has: " + strings.Join(have, ", ") + ")"
			}
			r.add(Error, "theme.txt",
				"font \""+want+"\" is not baked into any .pf2 in this theme", hint)
		}
	}
	if len(pf2s) == 0 {
		r.add(Warning, "theme.txt", "no .pf2 fonts shipped with this theme",
			"GRUB will fall back to its built-in font")
	}
}

// checkAssets cross-checks [assets] against theme.txt. The generated files are
// always correct; the two descriptions of them can still disagree.
func checkAssets(r *Result, src string) {
	a := r.Theme.Manifest.Assets
	if a == nil {
		return // a hand-made theme; nothing declared to cross-check
	}
	sel := a.Selection
	if sel.Style == "none" {
		return
	}

	fill, errFill := paint.Hex(sel.Fill)
	text, errText := paint.Hex(sel.Text)
	if errFill != nil {
		r.add(Error, "theme.toml", "assets.selection.fill: "+errFill.Error(), "")
	}
	if errText != nil {
		r.add(Error, "theme.toml", "assets.selection.text: "+errText.Error(), "")
	}

	// If these are close and the highlight ever fails to render, the selected
	// entry is unreadable with no clue why.
	if errFill == nil && errText == nil && fill.A > 0 && sel.Text != "" {
		switch ratio := paint.Contrast(fill, text); {
		case ratio < 3:
			r.add(Error, "theme.toml",
				fmt.Sprintf("selection text on the highlight has a contrast ratio of %.1f:1", ratio),
				"3:1 is the minimum for large text; pick a darker or lighter assets.selection.text")
		case ratio < 4.5:
			r.add(Warning, "theme.toml",
				fmt.Sprintf("selection text contrast is %.1f:1", ratio),
				"4.5:1 reads comfortably at any size")
		}
	}

	// theme.txt draws the text; theme.toml only describes it.
	if m := reSelectedColor.FindStringSubmatch(src); m != nil && sel.Text != "" && errText == nil {
		// Parsed, so "#fff" and "#FFFFFF" agree.
		if drawn, err := paint.Hex(m[1]); err == nil && drawn != text {
			r.add(Warning, "theme.txt",
				fmt.Sprintf("selected_item_color is %s but assets.selection.text is %s", m[1], sel.Text),
				"keep them equal, or the contrast check is measuring the wrong colour")
		}
	}

	// Matching heights keeps the highlight from being scaled.
	if sel.Height != nil {
		if m := reItemHeight.FindStringSubmatch(src); m != nil {
			if h, err := strconv.Atoi(m[1]); err == nil && h != *sel.Height {
				r.add(Warning, "theme.txt",
					fmt.Sprintf("item_height is %d but assets.selection.height is %d", h, *sel.Height),
					"keep them equal so the highlight is not scaled")
			}
		}
	}
}
