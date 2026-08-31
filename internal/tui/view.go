package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sarbojitrana/grub-themes/internal/paint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

const (
	listWidth  = 34
	minPreview = 28
)

// View draws the browser.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "loading…"
	}
	if m.mode == helping {
		return m.helpView()
	}
	if len(m.themes) == 0 {
		return m.emptyView()
	}

	header := m.header()
	footer := m.footer()
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	left := m.list(bodyHeight)
	body := left
	if m.width >= listWidth+minPreview {
		right := m.detail(m.width-listWidth-3, bodyHeight)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(left),
			dimSty.Render(strings.Repeat("│\n", bodyHeight-1)+"│"),
			lipgloss.NewStyle().PaddingLeft(1).Render(right),
		)
	}
	return header + "\n" + body + "\n" + footer
}

func (m Model) header() string {
	title := titleSty.Render(" GRUB THEMES")
	sub := dimSty.Render(fmt.Sprintf(" %d available", len(m.themes)))

	state := dimSty.Render("no theme applied")
	if m.statusErr != nil {
		state = lipgloss.NewStyle().Foreground(warnCol).Render("GRUB not detected: " + m.statusErr.Error())
	} else if m.status.Active != "" {
		state = badgeSty.Render("● " + m.status.Active + " is applied")
	}

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(sub) - lipgloss.Width(state) - 1
	if gap < 1 {
		gap = 1
	}
	return title + sub + strings.Repeat(" ", gap) + state
}

func (m Model) list(height int) string {
	var b strings.Builder
	// Keep the cursor visible on short terminals.
	start := 0
	if height > 0 && m.cursor >= height {
		start = m.cursor - height + 1
	}
	for i := start; i < len(m.themes) && i-start < height; i++ {
		t := m.themes[i]
		id := t.Manifest.Theme.ID

		marker := "  "
		nameStyle := lipgloss.NewStyle()
		if i == m.cursor {
			marker = selSty.Render("▸ ")
			nameStyle = selSty
		}
		label := fmt.Sprintf("%-12s %s", id, t.Manifest.Theme.Name)
		if len(label) > listWidth-6 {
			label = label[:listWidth-6]
		}
		line := marker + nameStyle.Render(label)
		if id == m.status.Active {
			line += badgeSty.Render(" ●")
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// detail is the preview image, then the manifest.
func (m Model) detail(cols, rows int) string {
	t, ok := m.selected()
	if !ok {
		return ""
	}
	info := m.info(t, cols)
	imgRows := rows - lipgloss.Height(info) - 1
	img := m.preview(t, cols, imgRows)
	if img == "" {
		return info
	}
	return img + "\n\n" + info
}

func (m Model) preview(t theme.Theme, cols, rows int) string {
	if rows < 3 {
		return ""
	}
	path := t.PreviewPath()
	if path == "" {
		return dimSty.Render("(this theme ships no preview image)")
	}
	key := fmt.Sprintf("%s|%dx%d", path, cols, rows)
	if s, ok := m.previews[key]; ok {
		return s
	}
	img, err := paint.Load(path)
	if err != nil {
		return dimSty.Render("(preview could not be read: " + err.Error() + ")")
	}
	s := renderImage(img, cols, rows)
	m.previews[key] = s // the map is shared; the model is copied by value
	return s
}

func (m Model) info(t theme.Theme, cols int) string {
	man := t.Manifest
	var b strings.Builder

	name := titleSty.Render(man.Theme.Name)
	if man.Theme.ID == m.status.Active {
		name += badgeSty.Render("  ● applied")
	}
	b.WriteString(name + "\n")
	b.WriteString(wrap(man.Theme.Description, cols) + "\n")

	meta := []string{}
	if man.Author.Name != "" {
		meta = append(meta, man.Author.Name)
	}
	if man.Theme.Version != "" {
		meta = append(meta, "v"+man.Theme.Version)
	}
	if man.Theme.License != "" {
		meta = append(meta, man.Theme.License)
	}
	if man.Display.DesignedFor != "" {
		meta = append(meta, man.Display.DesignedFor)
	}
	b.WriteString(dimSty.Render(strings.Join(meta, " · ")) + "\n")
	if len(man.Theme.Tags) > 0 {
		b.WriteString(dimSty.Render("tags: "+strings.Join(man.Theme.Tags, ", ")) + "\n")
	}
	b.WriteString(dimSty.Render(shorten(t.Dir, cols)))
	return b.String()
}

func (m Model) footer() string {
	var lines []string

	if m.status.ConsoleForce {
		lines = append(lines, lipgloss.NewStyle().Foreground(warnCol).Render(
			"! GRUB_TERMINAL_OUTPUT=console is set — no theme can render until that is off. Applying a theme fixes it."))
	}
	if m.message != "" {
		lines = append(lines, m.messageStyle.Render(m.message))
	}

	if m.mode == confirming {
		t, _ := m.selected()
		lines = append(lines, lipgloss.NewStyle().Foreground(warnCol).Render(
			fmt.Sprintf("Apply %s? This edits /etc/default/grub and regenerates grub.cfg (a backup is kept).  [y/N]",
				t.Manifest.Theme.Name)))
	} else {
		lines = append(lines, footerSty.Render(
			"↑↓ move · enter apply · r remove · l lint · p open preview · n build your own · q quit"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) emptyView() string {
	return "\n" + titleSty.Render("  No themes found.") + "\n\n" +
		"  grub-themes looks in, in order:\n" +
		dimSty.Render("    $GRUB_THEMES_DIR\n"+
			"    ./themes  (when you are in a checkout of the repository)\n"+
			"    ~/.local/share/grub-themes/themes\n"+
			"    /usr/share/grub-themes/themes\n") +
		"\n  Start one with " + selSty.Render("grub-themes new mytheme") + "\n\n" +
		footerSty.Render("  q quit")
}

func (m Model) helpView() string {
	steps := []struct{ cmd, what string }{
		{"grub-themes new mytheme", "scaffold a complete, lint-passing theme in ~/.local/share/grub-themes/themes"},
		{"$EDITOR .../mytheme/theme.toml", "colours, name, the selection style — no code, no ImageMagick"},
		{"$EDITOR .../mytheme/tools/background.svg", "the art. This is the part that is actually yours"},
		{"grub-themes build mytheme", "render the SVG, draw the pixmaps, bake the fonts — all correctly encoded"},
		{"grub-themes lint mytheme", "catch the silent failures before you reboot"},
		{"grub-themes preview mytheme", "see it as a PNG without booting anything"},
		{"grub-themes apply mytheme", "install it and regenerate grub.cfg, with rollback"},
	}

	var b strings.Builder
	b.WriteString("\n " + titleSty.Render("BUILD YOUR OWN THEME") + "\n\n")
	for _, s := range steps {
		b.WriteString("  " + selSty.Render(s.cmd) + "\n")
		b.WriteString("      " + dimSty.Render(s.what) + "\n")
	}
	b.WriteString("\n " + titleSty.Render("SHARE IT") + "\n\n")
	b.WriteString("  A theme you build locally is yours to keep — nothing has to be published.\n")
	b.WriteString("  If you would like it in the collection, copy the directory into\n")
	b.WriteString("  " + selSty.Render("themes/") + " in a fork of the repository and open a pull request:\n\n")
	// Newlines stay outside Render: lipgloss pads a styled block to its width.
	b.WriteString(dimSty.Render("      https://github.com/sarbojitrana/grub-themes") + "\n")
	b.WriteString(dimSty.Render("      see CONTRIBUTING.md and docs/build-your-own-theme.md") + "\n")
	b.WriteString("\n" + footerSty.Render(" any key to go back"))
	return b.String()
}

func wrap(s string, cols int) string {
	if cols < 10 || len(s) <= cols {
		return s
	}
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) > cols {
			out = append(out, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func shorten(path string, cols int) string {
	if cols < 12 || len(path) <= cols {
		return path
	}
	return "…" + path[len(path)-cols+1:]
}
