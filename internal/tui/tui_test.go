package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sarbojitrana/grub-themes/internal/theme"
)

func testThemes() []theme.Theme {
	var t theme.Theme
	t.Dir = "../../themes/jarvis"
	t.Manifest.Theme.ID = "jarvis"
	t.Manifest.Theme.Name = "J.A.R.V.I.S."
	t.Manifest.Theme.Description = "Arc-reactor HUD."
	t.Manifest.Files.Preview = "preview.png"
	return []theme.Theme{t}
}

// A frame must render at any size without panicking.
func TestViewRenders(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	for _, size := range [][2]int{{150, 44}, {80, 24}, {40, 12}} {
		m := New(testThemes(), "")
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		out := model.View()
		if !strings.Contains(out, "jarvis") {
			t.Errorf("%dx%d: frame does not mention the theme", size[0], size[1])
		}
	}
}

func TestHelpViewMentionsContributing(t *testing.T) {
	m := New(testThemes(), "")
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	out := model.View()
	for _, want := range []string{"grub-themes new", "pull request"} {
		if !strings.Contains(out, want) {
			t.Errorf("help view is missing %q", want)
		}
	}
}
