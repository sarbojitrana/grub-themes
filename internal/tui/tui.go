// Package tui is the theme browser: the thing you get when you run
// grub-themes with no arguments, or launch it from your application menu.
//
// It is a terminal UI rather than a desktop one on purpose. GRUB themes get
// applied on servers over SSH as often as on laptops, and a GTK dependency
// would make packaging the single static binary much harder. The previews are
// drawn as half-block colour cells, so you still see the theme.
package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sarbojitrana/grub-themes/internal/install"
	"github.com/sarbojitrana/grub-themes/internal/lint"
	"github.com/sarbojitrana/grub-themes/internal/theme"
)

var (
	accent    = lipgloss.AdaptiveColor{Light: "#0b6c8c", Dark: "#00d9ff"}
	dim       = lipgloss.AdaptiveColor{Light: "#5c6b78", Dark: "#7b8a99"}
	warnCol   = lipgloss.AdaptiveColor{Light: "#a35c00", Dark: "#ffcf1b"}
	errCol    = lipgloss.AdaptiveColor{Light: "#a4133c", Dark: "#ff5c7a"}
	okCol     = lipgloss.AdaptiveColor{Light: "#0b7a3b", Dark: "#4ade80"}
	titleSty  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimSty    = lipgloss.NewStyle().Foreground(dim)
	selSty    = lipgloss.NewStyle().Bold(true).Foreground(accent)
	badgeSty  = lipgloss.NewStyle().Foreground(okCol)
	panelSty  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(dim).Padding(0, 1)
	footerSty = lipgloss.NewStyle().Foreground(dim)
)

type mode int

const (
	browsing mode = iota
	confirming
	helping
)

// Model is the browser state.
type Model struct {
	themes    []theme.Theme
	cursor    int
	status    install.Status
	statusErr error

	width, height int
	mode          mode
	message       string
	messageStyle  lipgloss.Style
	previews      map[string]string // cache key: id + geometry

	themesDir string // passed through to the elevated helper
	quitting  bool
}

// New builds a browser over the given themes.
func New(themes []theme.Theme, themesDir string) Model {
	m := Model{
		themes:    themes,
		previews:  map[string]string{},
		themesDir: themesDir,
	}
	m.status, m.statusErr = install.Read()
	// Start on the theme that is currently applied: that is nearly always the
	// one you want to look at first.
	for i, t := range themes {
		if t.Manifest.Theme.ID == m.status.Active {
			m.cursor = i
		}
	}
	return m
}

// Run starts the browser.
func Run(themes []theme.Theme, themesDir string) error {
	p := tea.NewProgram(New(themes, themesDir), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd { return nil }

// ---------------------------------------------------------------- messages

type appliedMsg struct {
	id  string
	err error
}

type removedMsg struct{ err error }

func (m Model) selected() (theme.Theme, bool) {
	if m.cursor < 0 || m.cursor >= len(m.themes) {
		return theme.Theme{}, false
	}
	return m.themes[m.cursor], true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case appliedMsg:
		m.status, m.statusErr = install.Read()
		if msg.err != nil {
			m.setMessage("could not apply "+msg.id+": "+msg.err.Error(), errCol)
		} else {
			m.setMessage("applied "+msg.id+" — reboot to see it", okCol)
		}
		return m, nil

	case removedMsg:
		m.status, m.statusErr = install.Read()
		if msg.err != nil {
			m.setMessage("could not remove the theme: "+msg.err.Error(), errCol)
		} else {
			m.setMessage("theme removed, GRUB is back to its default", okCol)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) setMessage(s string, col lipgloss.AdaptiveColor) {
	m.message = s
	m.messageStyle = lipgloss.NewStyle().Foreground(col)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.mode == confirming {
		switch key {
		case "y", "Y", "enter":
			m.mode = browsing
			t, ok := m.selected()
			if !ok {
				return m, nil
			}
			m.setMessage("applying "+t.Manifest.Theme.ID+"…", warnCol)
			return m, m.applyCmd(t)
		default:
			m.mode = browsing
			m.setMessage("cancelled", dim)
			return m, nil
		}
	}

	if m.mode == helping {
		m.mode = browsing
		return m, nil
	}

	switch key {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.message = ""
		}
	case "down", "j":
		if m.cursor < len(m.themes)-1 {
			m.cursor++
			m.message = ""
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.themes) - 1

	case "enter":
		if _, ok := m.selected(); ok {
			m.mode = confirming
		}

	case "r":
		if m.status.Active == "" {
			m.setMessage("no theme is applied, so there is nothing to remove", dim)
			return m, nil
		}
		m.setMessage("removing "+m.status.Active+"…", warnCol)
		return m, m.removeCmd()

	case "l":
		if t, ok := m.selected(); ok {
			m.setMessage(lintSummary(t), lintColour(t))
		}

	case "p":
		if t, ok := m.selected(); ok {
			if p := t.PreviewPath(); p != "" {
				_ = exec.Command("xdg-open", p).Start()
				m.setMessage("opened "+p, dim)
			}
		}

	case "n", "?", "h":
		m.mode = helping
	}
	return m, nil
}

// applyCmd runs the install. Applying edits /etc/default/grub and regenerates
// grub.cfg, so it needs root: when the browser is not already root it hands
// off to sudo (or pkexec), which is also what puts the password prompt on a
// terminal the user can see.
func (m Model) applyCmd(t theme.Theme) tea.Cmd {
	id := t.Manifest.Theme.ID
	if os.Geteuid() == 0 {
		return func() tea.Msg {
			var log bytes.Buffer
			err := install.Apply(t, install.Options{Log: &log})
			return appliedMsg{id: id, err: err}
		}
	}
	// Pass the directory this theme actually came from: under sudo, root has
	// its own HOME and would not find a theme scaffolded in the user's data
	// directory.
	cmd, err := elevate("apply", id, filepath.Dir(t.Dir))
	if err != nil {
		return func() tea.Msg { return appliedMsg{id: id, err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return appliedMsg{id: id, err: err}
	})
}

func (m Model) removeCmd() tea.Cmd {
	if os.Geteuid() == 0 {
		return func() tea.Msg {
			var log bytes.Buffer
			return removedMsg{err: install.Remove(install.Options{Log: &log})}
		}
	}
	cmd, err := elevate("remove", "", m.themesDir)
	if err != nil {
		return func() tea.Msg { return removedMsg{err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return removedMsg{err: err} })
}

// elevate builds the command that re-runs this binary as root.
func elevate(sub, id, themesDir string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{exe, sub}
	if id != "" {
		args = append(args, id)
	}
	if themesDir != "" {
		args = append(args, "--themes-dir", themesDir)
	}
	args = append(args, "--pause")

	if sudo, err := exec.LookPath("sudo"); err == nil {
		return exec.Command(sudo, args...), nil
	}
	if pk, err := exec.LookPath("pkexec"); err == nil {
		return exec.Command(pk, args...), nil
	}
	return nil, fmt.Errorf("neither sudo nor pkexec found; run: sudo %s %s %s", exe, sub, id)
}

func lintSummary(t theme.Theme) string {
	res := lint.Check(t)
	if len(res.Findings) == 0 {
		return "lint: no problems"
	}
	errs, warns := 0, 0
	for _, f := range res.Findings {
		if f.Severity == lint.Error {
			errs++
		} else {
			warns++
		}
	}
	first := res.Findings[0]
	return fmt.Sprintf("lint: %d error(s), %d warning(s) — %s: %s",
		errs, warns, first.File, first.Message)
}

func lintColour(t theme.Theme) lipgloss.AdaptiveColor {
	res := lint.Check(t)
	switch {
	case res.HasErrors():
		return errCol
	case len(res.Findings) > 0:
		return warnCol
	default:
		return okCol
	}
}
