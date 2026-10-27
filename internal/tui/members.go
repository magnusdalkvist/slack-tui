package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// `:members`: who is in the open conversation, filtered as you type;
// enter shows the person's profile.

type membersState struct {
	conv     string
	ids      []string // nil while loading
	input    textinput.Model
	matches  []int // indexes into ids, best first
	sel, top int
}

type membersMsg struct {
	conv string
	ids  []string
	err  error
}

func (m *Model) openMembers() tea.Cmd {
	c := m.convs[m.current]
	if c == nil {
		return m.setStatus("open a conversation first", false)
	}
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "filter people"
	ti.CharLimit = 80
	m.closeSearch()
	m.pins, m.sched, m.shared, m.chans = nil, nil, nil, nil
	m.insert = false
	m.input.Blur()
	m.members = &membersState{conv: c.ID, input: ti}
	id := c.ID
	return tea.Batch(m.members.input.Focus(), func() tea.Msg {
		ids, err := m.client.Members(m.ctx, id)
		return membersMsg{conv: id, ids: ids, err: err}
	})
}

func (m *Model) membersResult(msg membersMsg) tea.Cmd {
	s := m.members
	if s == nil || s.conv != msg.conv {
		return nil
	}
	if msg.err != nil {
		m.members = nil
		return m.setStatus("members: "+slackReason(msg.err), true)
	}
	// By name, as Slack lists them.
	slices.SortStableFunc(msg.ids, func(a, b string) int {
		return strings.Compare(strings.ToLower(m.memberName(a)), strings.ToLower(m.memberName(b)))
	})
	s.ids = msg.ids
	m.filterMembers()
	return nil
}

func (m *Model) memberName(id string) string {
	if u, ok := m.users[id]; ok {
		return u.Label()
	}
	return id
}

func (m *Model) filterMembers() {
	s := m.members
	q := strings.TrimPrefix(strings.TrimSpace(s.input.Value()), "@")
	s.sel, s.top = 0, 0
	s.matches = s.matches[:0]
	if q == "" {
		for i := range s.ids {
			s.matches = append(s.matches, i)
		}
		return
	}
	s.matches = rank(q, len(s.ids),
		func(i int) []string {
			u := m.users[s.ids[i]]
			return []string{u.Label(), u.RealName, u.Name, u.Title}
		},
		func(i int) int { return len(m.memberName(s.ids[i])) })
}

func (m *Model) membersKey(k tea.KeyMsg) tea.Cmd {
	s := m.members
	n := len(s.matches)
	page := max(1, m.height/2)
	switch k.String() {
	case "esc":
		m.members = nil
		return nil
	case "enter":
		if n == 0 {
			return nil
		}
		return m.openProfile(s.ids[s.matches[min(s.sel, n-1)]])
	case "down", "ctrl+n", "ctrl+j", "tab":
		s.sel++
	case "up", "ctrl+p", "shift+tab":
		s.sel--
	case "pgdown", "ctrl+d":
		s.sel += page
	case "pgup", "ctrl+u":
		s.sel -= page
	default:
		before := s.input.Value()
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(k)
		if s.input.Value() != before {
			m.filterMembers()
		}
		return cmd
	}
	s.sel = min(max(s.sel, 0), max(n-1, 0))
	return nil
}

// renderMembers is the Members pane: presence, name, title; the filter in
// the composer frame.
func (m *Model) renderMembers(w, h int) string {
	s := m.members
	lw := m.paneListW(w, true)
	s.input.Width = max(10, lw-4)
	p := paneSpec{title: frameTitle("Members", true), top: &s.top,
		input: paneInput(s.input.View(), "", true), inputBorder: sty.OK}
	if c := m.convs[s.conv]; c != nil {
		p.title = frameTitle("Members", true, sty.Muted.Render(m.convRef(c)))
	}
	if s.ids != nil {
		p.right = sty.Faint.Render(fmt.Sprintf("%d %s", len(s.ids), plural(len(s.ids), "person", "people")))
	}
	p.empty = "nobody matches"
	if s.ids == nil {
		p.empty = "loading…"
	}
	if len(s.matches) == 0 {
		return m.renderPane(p, w, h)
	}
	q := strings.TrimSpace(s.input.Value())
	sel := min(max(s.sel, 0), len(s.matches)-1)
	p.selStart, p.selEnd = sel, sel+1
	for i, idx := range s.matches {
		id := s.ids[idx]
		u := m.users[id]
		dot := sty.Faint.Render("○")
		if m.presence[id] {
			dot = sty.OK.Render(onlineDot)
		}
		row := dot + " " + markMatch(m.memberName(id), q, sty.Normal)
		if id == m.client.SelfID {
			row += " " + sty.Faint.Render("(you)")
		}
		if e := m.statusEmoji(id); e != "" {
			row += " " + e
		}
		var info []string
		if u.Title != "" {
			info = append(info, u.Title)
		}
		if m.extUsers[id] {
			info = append(info, "external")
		}
		if u.IsBot {
			info = append(info, "app")
		}
		if len(info) > 0 {
			row += "  " + sty.Faint.Render(strings.Join(info, " · "))
		}
		p.lines = append(p.lines, selLine(row, lw, i == sel, true))
	}
	id := s.ids[s.matches[sel]]
	p.preview = func(w, h int) string {
		u := m.users[id]
		out := []string{sty.Title.Render(m.memberName(id))}
		if u.Title != "" {
			out = append(out, sty.Muted.Render(u.Title))
		}
		out = append(out, "")
		field := func(label, value string) {
			if value != "" {
				out = append(out, sty.Faint.Render(fmt.Sprintf("%-12s", label))+value)
			}
		}
		field("Status", m.statusLine(id))
		field("Local time", localTime(u, clockNow(), m.cfg.TimeFormat))
		field("Email", u.Email)
		return strings.Join(out[:min(len(out), h)], "\n")
	}
	return m.renderPane(p, w, h)
}

// memberIDs are people in the open members list that users.list didn't
// have (other organisations), for scanUsers to look up.
func (m *Model) memberIDs() []string {
	if m.members == nil {
		return nil
	}
	return m.members.ids
}
