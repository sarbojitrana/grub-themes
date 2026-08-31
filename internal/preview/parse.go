// Package preview renders a theme.txt to a PNG, so a theme can be reviewed
// without installing GRUB or rebooting anything.
//
// A layout check, not an emulator: the geometry, colours, pixmaps and fonts
// are the theme's real ones, but GRUB's renderer is not reimplemented.
package preview

import (
	"strconv"
	"strings"
)

// component is one `+ name { ... }` block from theme.txt.
type component struct {
	Name  string
	Props map[string]string
}

// document is a parsed theme.txt.
type document struct {
	Globals    map[string]string
	Components []component
}

func (d document) global(key string) string { return d.Globals[key] }

func (c component) get(key string) string { return c.Props[key] }

func (c component) getOr(key, def string) string {
	if v, ok := c.Props[key]; ok && v != "" {
		return v
	}
	return def
}

func (c component) num(key string, def int) int {
	v, ok := c.Props[key]
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// parse reads theme.txt: `key: value` at the top level, `+ name { key = value }`
// blocks below, `#` comments.
func parse(src string) document {
	doc := document{Globals: map[string]string{}}
	var cur *component
	depth := 0

	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "+") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "+"))
			name = strings.TrimSpace(strings.TrimSuffix(name, "{"))
			doc.Components = append(doc.Components, component{Name: name, Props: map[string]string{}})
			cur = &doc.Components[len(doc.Components)-1]
			depth++
			continue
		}
		if line == "}" {
			depth--
			if depth <= 0 {
				depth = 0
				cur = nil
			}
			continue
		}

		key, val, ok := splitAssign(line)
		if !ok {
			continue
		}
		if cur != nil {
			cur.Props[key] = val
		} else {
			doc.Globals[key] = val
		}
	}
	return doc
}

func stripComment(s string) string {
	// A '#' inside quotes is a colour, not a comment.
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return s[:i]
			}
		}
	}
	return s
}

func splitAssign(line string) (key, val string, ok bool) {
	i := strings.IndexAny(line, ":=")
	if i < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	val = strings.TrimSpace(line[i+1:])
	val = strings.TrimSuffix(val, "{")
	val = strings.TrimSpace(val)
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = val[1 : len(val)-1]
	}
	return key, val, key != ""
}

// dim resolves a GRUB dimension: "44", "6%", "100%-58", "50%+20".
func dim(s string, extent int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	sign, rest := 1, s
	var offset int
	if i := strings.IndexAny(s[1:], "+-"); i >= 0 {
		i++
		rest = s[:i]
		if s[i] == '-' {
			sign = -1
		}
		offset, _ = strconv.Atoi(strings.TrimSpace(s[i+1:]))
	}
	rest = strings.TrimSpace(rest)
	base := 0
	if strings.HasSuffix(rest, "%") {
		p, err := strconv.ParseFloat(strings.TrimSuffix(rest, "%"), 64)
		if err == nil {
			base = int(p / 100 * float64(extent))
		}
	} else {
		base, _ = strconv.Atoi(rest)
	}
	return base + sign*offset
}
