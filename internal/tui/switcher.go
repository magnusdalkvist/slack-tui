package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

// switcher is the Ctrl-K fuzzy conversation finder.
type switcher struct {
	input   textinput.Model
	ids     []string // all candidates, in sidebar order
	labels  []string
	matches []string // ids matching the query, best first
	sel     int
	// convCount: ids past it are people we have no DM with
	// (personPrefix+user ID), offered only once something is typed.
	convCount int
}

func (m *Model) newSwitcher() *switcher {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "jump to a channel or person"
	ti.Focus()
	s := &switcher{input: ti}
	// Every conversation, including those in folded sections.
	s.ids = append(s.ids, unreadsID)
	s.labels = append(s.labels, "Unreads")
	for _, sec := range m.sections() {
		for _, id := range sec.ids {
			c := m.convs[id]
			label := m.convLabel(c)
			if !c.IsDM() {
				label = "#" + label
			}
			s.ids = append(s.ids, id)
			s.labels = append(s.labels, label)
		}
	}
	m.switcherPeople(s)
	s.filter()
	return s
}

func (s *switcher) filter() {
	q := strings.TrimSpace(s.input.Value())
	s.matches = s.matches[:0]
	if q == "" {
		s.matches = append(s.matches, s.ids[:s.convCount]...)
	} else {
		for _, i := range fuzzyRank(q, s.labels) {
			s.matches = append(s.matches, s.ids[i])
		}
	}
	s.sel = 0
}

func (m *Model) switcherKey(k tea.KeyMsg) tea.Cmd {
	s := m.switcher
	switch k.String() {
	case "esc", "ctrl+k":
		m.switcher = nil
		return nil
	case "enter":
		m.switcher = nil
		if len(s.matches) > 0 {
			if uid, ok := strings.CutPrefix(s.matches[s.sel], personPrefix); ok {
				return m.openDMWith([]string{uid})
			}
			return m.open(s.matches[s.sel])
		}
		return nil
	case "down", "ctrl+n", "ctrl+j", "tab":
		if s.sel < len(s.matches)-1 {
			s.sel++
		}
		return nil
	case "up", "ctrl+p", "shift+tab":
		if s.sel > 0 {
			s.sel--
		}
		return nil
	}
	var cmd tea.Cmd
	before := s.input.Value()
	s.input, cmd = s.input.Update(k)
	if s.input.Value() != before {
		s.filter()
	}
	return cmd
}

// renderSwitcher is the Jump to modal: icon, name with the query's
// match marked, and what it is on the right.
func (m *Model) renderSwitcher() string {
	s := m.switcher
	w := m.modalWidth(76)
	inner := w - 2
	s.input.Width = inner - 4
	q := strings.TrimSpace(s.input.Value())
	rows := []string{" " + s.input.View(), modalRule(inner)}
	listH := max(3, min(12, m.height/2))
	start := max(0, s.sel-listH+1)
	for i := start; i < len(s.matches) && i < start+listH; i++ {
		icon, name, meta, style := m.switchRow(s.matches[i])
		row := " " + slot(icon) + markMatch(name.label, q, style)
		if name.extra != "" {
			row += " " + sty.Faint.Render(name.extra)
		}
		rows = append(rows, modalRow(rightAlign(row, sty.Faint.Render(meta), inner), inner, i == s.sel))
	}
	if len(s.matches) == 0 {
		rows = append(rows, " "+sty.Muted.Render("no matches"))
	}
	right := fmt.Sprintf("%d of %d", len(s.matches), len(s.ids))
	return modalBox(w, "Jump to", right, rows)
}

type switchName struct{ label, extra string }

// switchRow is a switcher entry's icon, name (and a faint extra: a
// person's real name), meta and style.
func (m *Model) switchRow(id string) (icon string, name switchName, meta string, style lipgloss.Style) {
	style = sty.Muted
	if uid, ok := strings.CutPrefix(id, personPrefix); ok {
		u := m.users[uid]
		name = switchName{label: u.Label()}
		if u.RealName != "" && u.RealName != u.Label() {
			name.extra = u.RealName
		}
		return m.personIcon(uid), name, "person", style
	}
	c := m.convs[id]
	if c == nil {
		n := m.unreadCount()
		if n > 0 {
			style = sty.Unread
		}
		return style.Render(ico.Unreads), switchName{label: "Unreads"}, fmt.Sprintf("%d %s", n, plural(n, "conversation", "conversations")), style
	}
	if c.Unread() {
		style = sty.Unread
	}
	name = switchName{label: m.convLabel(c)}
	switch c.Kind {
	case backend.KindDM:
		icon = m.personIcon(c.UserID)
		switch {
		case c.UserID == m.client.SelfID:
			name.label, meta = strings.TrimSuffix(name.label, " (you)"), "you"
		case m.isActive(c.UserID):
			meta = "DM · active"
		default:
			meta = "DM · away"
		}
	case backend.KindGroup:
		icon, meta = style.Render(ico.Group), "group DM"
	case backend.KindPrivate:
		icon, meta = style.Render(ico.Private), "private channel"
	default:
		icon, meta = style.Render(ico.Channel), "channel"
	}
	if c.Mentions > 0 {
		meta = sty.Err.Render(fmt.Sprintf("%d %s", c.Mentions, plural(c.Mentions, "mention", "mentions"))) + sty.Faint.Render(" · "+meta)
	} else if c.Unread() {
		meta = "new · " + meta
	}
	return icon, name, meta, style
}

// personIcon is a person's icon, showing presence: solid in ok when
// active, outline in faint when away.
func (m *Model) personIcon(uid string) string {
	if m.isActive(uid) {
		return sty.OK.Render(ico.UserActive)
	}
	return sty.Faint.Render(ico.UserAway)
}
