package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/config"
)

// Startup and error screens (DESIGN.md §11). Both keep the status bar.

// renderLoading is the start: the team, then what is being loaded, each
// step ✓ when done, the spinner while running, · while waiting.
func (m *Model) renderLoading(h int) string {
	step := func(done, running bool, label string) string {
		switch {
		case done:
			return sty.OK.Render("✓") + " " + sty.Normal.Render(label)
		case running:
			return sty.Bar.Render(m.spin.View()) + " " + sty.Normal.Render(label)
		}
		return sty.Faint.Render("· " + label)
	}
	cache := "cache loaded"
	if m.cacheChecked && m.cacheAge.IsZero() {
		cache = "no cache yet"
	}
	rows := []string{
		sty.Title.Render(m.client.TeamName),
		"",
		step(m.cacheChecked, !m.cacheChecked, cache),
		step(m.conn == "connected", m.conn != "connected", "connecting"),
		step(m.booted, m.cacheChecked, "channels & people"),
	}
	return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// loadingBar is the status bar while starting: where the cache is from.
func (m *Model) loadingBar() string {
	left := " " + sty.Faint.Render("slack-tui")
	if !m.cacheAge.IsZero() {
		left += sty.Faint.Render(" · cache from " + relTime(m.cacheAge, m.cfg.TimeFormat))
	}
	return m.bar(left, []Hint{{"q", "quit"}}, false)
}

// renderFatal is a start that failed with nothing cached to show: why,
// and where the config is.
func (m *Model) renderFatal(h int) string {
	reason := m.bootErr.Error()
	w := min(m.width-4, 72)
	rows := []string{sty.Err.Bold(true).Render("✕ Couldn't start"), ""}
	for _, l := range wrap(reason, w) {
		rows = append(rows, sty.Muted.Render(ansi.Strip(l)))
	}
	rows = append(rows, "", sty.Faint.Render(config.Path()))
	return lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, lipgloss.JoinVertical(lipgloss.Left, rows...))
}

type configEditedMsg struct{ err error }

// editConfig is e on the error screen: the config in $EDITOR, then
// another try at starting.
func (m *Model) editConfig() tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	argv := strings.Fields(editor)
	cmd := exec.Command(argv[0], append(argv[1:], config.Path())...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return configEditedMsg{err} })
}
