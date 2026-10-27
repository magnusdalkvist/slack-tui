package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The status bar (DESIGN.md §8): one row on raised, always there.
// [mode pill] [connection] [your status] [message …]        [hints]
// Key hints live here and nowhere else.

// Hint is one "key label" group on the right of the status bar.
type Hint struct{ Key, Label string }

const maxHints = 5

// renderHints lays hints out as "key label   key label", at most five
// groups and no wider than width, with "? help" last when it fits and
// withHelp (not while typing, where ? is a character).
func renderHints(hs []Hint, width int, withHelp bool) string {
	render := func(hs []Hint) string {
		parts := make([]string, len(hs))
		for i, h := range hs {
			parts[i] = sty.Key.Render(h.Key) + " " + sty.Faint.Render(h.Label)
		}
		return strings.Join(parts, "   ")
	}
	limit := maxHints
	if withHelp && !hasKey(hs, "?") {
		limit-- // room for "? help"
	}
	var out []Hint
	for _, h := range hs {
		if len(out) == limit || lipgloss.Width(render(append(out, h))) > width {
			break
		}
		out = append(out, h)
	}
	if help := (Hint{"?", "help"}); limit < maxHints && lipgloss.Width(render(append(out, help))) <= width {
		out = append(out, help)
	}
	return render(out)
}

func hasKey(hs []Hint, k string) bool {
	for _, h := range hs {
		if h.Key == k {
			return true
		}
	}
	return false
}

// pill is a mode pill: bold onAccent text on a solid colour.
func (m *Model) pill() string {
	switch {
	case m.bootErr != nil:
		return sty.PillErr.Render("ERROR")
	case m.cmd != nil:
		return sty.PillCmd.Render(":")
	case m.confirm != nil:
		return sty.PillWarn.Render("CONFIRM")
	case m.viewer != nil:
		return sty.ModeNormal.Render("VIEW")
	case m.switcher != nil:
		return sty.ModeNormal.Render("JUMP")
	case m.picker != nil:
		return sty.ModeNormal.Render("REACT")
	case m.profile != nil:
		return sty.ModeNormal.Render("PROFILE")
	case m.opener != nil:
		return sty.ModeNormal.Render("OPEN")
	case m.search != nil && (m.search.open || m.search.typing):
		return sty.PillWarn.Render("SEARCH")
	case m.insert:
		return sty.ModeInsert.Render("INSERT")
	}
	return sty.ModeNormal.Render("NORMAL")
}

// connLine is the connection state: live, reconnecting, or the offline chip.
func (m *Model) connLine() string {
	switch {
	case m.isOffline():
		line := sty.PillErr.Render(ico.Offline + " offline")
		if m.offline {
			line += " " + sty.Muted.Render("showing cached messages")
			if left := m.retryAt.Sub(clockNow()).Round(time.Second); left > 0 {
				line += sty.Muted.Render(fmt.Sprintf(" · retry in %ds", int(left.Seconds())))
			}
		}
		return line
	case m.conn == "connected":
		return sty.OK.Render("● live")
	}
	return sty.Warn.Render("◌ reconnecting…")
}

// isOffline: the start failed and the cache is showing, or the live
// connection is gone for good.
func (m *Model) isOffline() bool { return m.offline || m.conn == "error" }

// meStatus is the user's own state: status, Do Not Disturb, away.
func (m *Model) meStatus() []string {
	var parts []string
	if line := m.statusLine(m.client.SelfID); line != "" {
		parts = append(parts, sty.Muted.Render(ansi.Truncate(line, 32, "…")))
	}
	if until, on := m.dnd.Until(clockNow()); on {
		parts = append(parts, sty.Faint.Render(ico.DND+" dnd · "+shortDur(until.Sub(clockNow()))))
	}
	if m.away {
		parts = append(parts, sty.Faint.Render("○ away"))
	}
	return parts
}

// shortDur is "45m", "2h", "1h30m", "3d".
func shortDur(d time.Duration) string {
	d = d.Round(time.Minute)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", max(1, int(d.Minutes())))
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// busy is true while something runs that the bar shows a spinner for.
func (m *Model) busy() bool { return m.infoPending > 0 }

// spinIfBusy starts the spinner when there is progress to show.
func (m *Model) spinIfBusy() tea.Cmd {
	if !m.busy() || m.spinning {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

type offlineTick struct{}

// offlineTicker counts the retry down, once a second while offline.
func offlineTicker() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return offlineTick{} })
}

// retryNow is R while offline: start again, or reconnect live events.
func (m *Model) retryNow() tea.Cmd {
	if m.offline {
		m.retryAt = clockNow()
		return tea.Batch(m.boot(), m.setStatus("retrying…", false))
	}
	if m.conn == "error" {
		m.conn = "connecting"
		go m.client.Listen(m.ctx, m.events)
	}
	return nil
}

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot))
}

// message is the middle zone's transient part: a y/n question, progress,
// or the last result or error.
func (m *Model) message() string {
	if c := m.confirm; c != nil {
		s := sty.Normal.Render(c.prompt)
		if c.detail != "" {
			s += "  " + sty.Faint.Render(c.detail)
		}
		return s
	}
	var parts []string
	if m.busy() {
		parts = append(parts, sty.Bar.Render(m.spin.View())+" "+sty.Muted.Render("checking unreads")+"  "+
			sty.Faint.Render(fmt.Sprintf("%d of %d channels", m.infoTotal-m.infoPending, m.infoTotal)))
	}
	switch {
	case m.status == "":
	case m.statusErr:
		parts = append(parts, sty.Err.Render("✕ "+m.status))
	case m.statusOK:
		parts = append(parts, sty.OK.Render("✓ "+m.status))
	default:
		parts = append(parts, sty.Muted.Render(m.status))
	}
	return strings.Join(parts, "   ")
}

func (m *Model) renderStatus() string {
	if m.cmd != nil && !m.cmd.inPane {
		return m.renderCmdLine()
	}
	left := []string{m.pill()}
	if m.confirm == nil {
		left = append(left, m.connLine())
		left = append(left, m.meStatus()...)
	}
	if msg := m.message(); msg != "" {
		left = append(left, msg)
	}
	typing := m.insert || m.cmd != nil || m.confirm != nil || m.switcher != nil || m.picker != nil || m.help != nil ||
		m.chans != nil || m.members != nil || (m.search != nil && m.search.typing)
	return m.bar(strings.Join(left, "  "), m.hints(), !typing)
}

// bar lays out the status row on raised: left as given, the hints on
// the right in what room is left.
func (m *Model) bar(left string, hints []Hint, withHelp bool) string {
	left = ansi.Truncate(left, m.width-1, "…")
	room := m.width - lipgloss.Width(left) - 3
	right := ""
	if room > 8 {
		right = renderHints(hints, room, withHelp)
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	return paintBg(left+strings.Repeat(" ", gap)+right+" ", m.width, sty.RaisedBgOn())
}

// renderCmdLine is the command line: the whole bar becomes its input.
func (m *Model) renderCmdLine() string {
	m.cmd.input.Width = max(10, m.width/2)
	return m.bar(sty.PillCmd.Render(":")+" "+m.cmd.input.View(), []Hint{
		{"tab", "complete"}, {"↑↓", "history"}, {"⏎", "run"}, {"esc", "cancel"},
	}, false)
}

// hints are the keys for what has focus now, most useful first.
func (m *Model) hints() []Hint {
	var hs []Hint
	if m.isOffline() {
		hs = append(hs, Hint{"R", "retry now"})
	}
	switch {
	case m.cmd != nil:
		return []Hint{{"⏎", "save"}, {"esc", "cancel"}}
	case m.confirm != nil:
		return []Hint{{"y", m.confirm.yesLabel}, {"n", m.confirm.noLabel}}
	case m.bootErr != nil:
		return []Hint{{"e", "edit config"}, {"q", "quit"}}
	case m.viewer != nil:
		return append(hs, m.viewer.Hints()...)
	case m.switcher != nil:
		return append(hs, m.switcher.Hints()...)
	case m.picker != nil:
		return append(hs, m.picker.Hints()...)
	case m.profile != nil:
		return append(hs, m.profile.Hints(m)...)
	case m.opener != nil:
		return append(hs, m.opener.Hints()...)
	case m.help != nil:
		return append(hs, m.help.Hints()...)
	case m.search != nil && m.search.open:
		return append(hs, m.search.Hints()...)
	case m.pins != nil:
		return append(hs, m.pins.Hints()...)
	case m.sched != nil:
		return append(hs, m.sched.Hints()...)
	case m.chans != nil:
		return append(hs, m.chans.Hints()...)
	case m.members != nil:
		return append(hs, m.members.Hints()...)
	case m.shared != nil:
		return append(hs, m.shared.Hints()...)
	case m.insert:
		return append(hs, m.insertHints()...)
	}
	switch {
	case m.focus == fSidebar:
		hs = append(hs, Hint{"⏎", "open"}, Hint{"z", "fold"}, Hint{"u", "next unread"}, Hint{"ctrl+k", "jump"})
	case m.current == unreadsID:
		hs = append(hs, Hint{"⏎", "open"}, Hint{"r", "mark read"}, Hint{"u", "next unread"})
		if m.stacked() {
			hs = append(hs, Hint{"h", "back"})
		}
	case m.focus == fThread && m.thread != nil:
		hs = append(hs, Hint{"esc", "back"}, Hint{"i", "reply"}, Hint{"+", "react"}, Hint{"x", "close"})
	case m.stacked():
		hs = append(hs, Hint{"esc", "back"}, Hint{"t", "thread"}, Hint{"i", "write"}, Hint{"+", "react"})
	default:
		if l := m.lists[m.current]; l != nil && l.hasMore && l.sel == 0 {
			hs = append(hs, Hint{"k", "older messages"})
		}
		if m.statusOK && m.savedPath != "" && m.status != "" {
			hs = append(hs, Hint{"o", "open it"})
		}
		if len(m.failedImages()) > 0 {
			hs = append(hs, Hint{"r", "retry"})
		}
		hs = append(hs, Hint{"i", "write"}, Hint{"⏎", "thread"}, Hint{"+", "react"}, Hint{"/", "search"})
	}
	return hs
}

func (m *Model) insertHints() []Hint {
	switch {
	case m.complete != nil:
		return []Hint{{"tab", "insert"}, {"↑↓", "choose"}, {"esc", "close"}}
	case m.editing != nil:
		return []Hint{{"⏎", "save"}, {"esc", "cancel"}}
	}
	hs := []Hint{{"⏎", "send"}, {"alt+⏎", "new line"}}
	if len(m.attach) > 0 {
		hs = append(hs, Hint{"ctrl+←→", "pick"}, Hint{"ctrl+x", "drop"})
	} else if _, thread := m.inputTarget(); thread != "" {
		hs = append(hs, Hint{"ctrl+o", "also to channel"})
	} else {
		hs = append(hs, Hint{"ctrl+v", "image"})
	}
	return append(hs, Hint{"esc", "normal"})
}

func (*switcher) Hints() []Hint {
	return []Hint{{"⏎", "open"}, {"↑↓", "move"}, {"esc", "close"}}
}

func (*picker) Hints() []Hint {
	return []Hint{{"⏎", "react / un-react"}, {"↑↓", "move"}, {"esc", "close"}}
}

func (p *profileState) Hints(m *Model) []Hint {
	hs := []Hint{{"⏎", "message"}}
	if m.users[p.id].Email != "" {
		hs = append(hs, Hint{"c", "copy email"})
	}
	return append(hs, Hint{"esc", "close"})
}

func (*opener) Hints() []Hint {
	return []Hint{{"1-9", "open"}, {"⏎", "open"}, {"esc", "close"}}
}

func (*helpState) Hints() []Hint {
	return []Hint{{"⏎", "run command"}, {"↑↓", "move"}, {"esc", "close"}}
}

func (s *searchState) Hints() []Hint {
	if s.typing {
		return []Hint{{"⏎", "search"}, {"esc", "back"}}
	}
	return []Hint{{"⏎", "go to message"}, {"o", "open in Slack"}, {"n N", "next / prev"}, {"/", "edit query"}, {"esc", "close"}}
}

func (*pinsState) Hints() []Hint {
	return []Hint{{"⏎", "go to / open"}, {"o", "in Slack"}, {"p", "unpin"}, {"a", "add bookmark"}, {"esc", "close"}}
}

func (*schedState) Hints() []Hint {
	return []Hint{{"⏎", "go to"}, {"e", "edit"}, {"d", "cancel it"}, {"R", "reload"}, {"esc", "close"}}
}

func (*chansState) Hints() []Hint {
	return []Hint{{"⏎", "join / open"}, {"↑↓", "move"}, {"esc", "close"}}
}

func (*membersState) Hints() []Hint {
	return []Hint{{"⏎", "profile"}, {"↑↓", "move"}, {"esc", "close"}}
}

func (*sharedState) Hints() []Hint {
	return []Hint{{"⏎", "go to message"}, {"o", "open"}, {"s", "save"}, {"v", "view"}, {"tab", "files / links"}}
}

func (*viewer) Hints() []Hint {
	return []Hint{{"← →", "browse"}, {"o", "open"}, {"s", "save"}, {"i", "details"}, {"esc", "close"}}
}
