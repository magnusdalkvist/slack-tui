package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

// A person's profile card: `I` on a message (its sender) or a DM, or
// `:profile @person`. Title, local time, status, email; enter writes to them.

type profileState struct {
	id      string
	loading bool
}

type profileMsg struct {
	user backend.User
	err  error
}

// openProfile shows what we know of a person at once and fetches the rest:
// users.list can be a day old, and leaves out people from other
// organisations.
func (m *Model) openProfile(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	m.profile = &profileState{id: id, loading: true}
	var cmd tea.Cmd
	if _, known := m.presence[id]; !known && id != m.client.SelfID && m.cfg.PresenceMinutes >= 0 {
		cmd = m.fetchPresence([]string{id}, false)
	}
	return tea.Batch(cmd, func() tea.Msg {
		u, err := m.client.UserInfo(m.ctx, id)
		return profileMsg{user: u, err: err}
	})
}

func (m *Model) profileResult(msg profileMsg) tea.Cmd {
	if p := m.profile; p != nil && (msg.err != nil || p.id == msg.user.ID) {
		p.loading = false
	}
	if msg.err != nil {
		return m.setStatus("profile: "+msg.err.Error(), true)
	}
	m.users[msg.user.ID] = msg.user
	return nil
}

// cmdProfile opens the profile of the person named, or of the one this
// DM is with.
func (m *Model) cmdProfile(arg string) tea.Cmd {
	if arg = strings.TrimSpace(arg); arg != "" {
		u, err := m.userByHandle(arg)
		if err != nil && u.ID != m.client.SelfID {
			return m.setStatus(err.Error(), true)
		}
		return m.openProfile(u.ID)
	}
	if c := m.convs[m.current]; c != nil && c.Kind == backend.KindDM {
		return m.openProfile(c.UserID)
	}
	return m.setStatus("usage: :profile @person (or I on a message)", false)
}

// profileFromSidebar is `I` in the sidebar: the person of the DM under
// the cursor.
func (m *Model) profileFromSidebar() tea.Cmd {
	if m.sideCur < len(m.side) {
		if c := m.convs[m.side[m.sideCur].id]; c != nil && c.Kind == backend.KindDM {
			return m.openProfile(c.UserID)
		}
	}
	return m.setStatus("I shows a person's profile: select a message or a DM", false)
}

// profileOfMessage is `I` on a message: its sender, or in a DM the other
// person when the message is our own.
func (m *Model) profileOfMessage(msg backend.Message) tea.Cmd {
	if msg.User == "" {
		return m.setStatus("apps have no profile", false)
	}
	if c := m.convs[m.current]; msg.User == m.client.SelfID && c != nil && c.Kind == backend.KindDM {
		return m.openProfile(c.UserID)
	}
	return m.openProfile(msg.User)
}

func (m *Model) profileKey(k tea.KeyMsg) tea.Cmd {
	id := m.profile.id
	switch k.String() {
	case "esc", "q", "I":
		m.profile = nil
	case "enter", "i":
		m.profile = nil
		return m.openDMWith([]string{id})
	case "c":
		return m.copyText(m.users[id].Email)
	}
	return nil
}

// renderProfile is the Profile modal: who, their title, then the
// fields in a 12-cell label column.
func (m *Model) renderProfile() string {
	p := m.profile
	u := m.users[p.id]
	w := m.modalWidth(64)
	inner := w - 2

	name := u.RealName
	if name == "" {
		name = u.Label()
	}
	if name == "" {
		name = p.id
	}
	dot := sty.Faint.Render("○")
	if m.presence[p.id] {
		dot = sty.OK.Render(onlineDot)
	}
	head := " " + dot + " " + sty.Title.Render(name)
	var sub []string
	if u.DisplayName != "" && u.DisplayName != name {
		sub = append(sub, u.DisplayName)
	}
	if u.Pronouns != "" {
		sub = append(sub, u.Pronouns)
	}
	if len(sub) > 0 {
		head += "  " + sty.Faint.Render(strings.Join(sub, " · "))
	}
	if m.extUsers[p.id] {
		head += "  " + sty.AppTag.Render("EXTERNAL")
	}
	rows := []string{head}
	if u.Title != "" {
		rows = append(rows, "   "+sty.Muted.Render(u.Title))
	}
	rows = append(rows, modalRule(inner))

	field := func(label, value string) {
		if value != "" {
			rows = append(rows, " "+sty.Faint.Render(fmt.Sprintf("%-12s", label))+value)
		}
	}
	if line := m.statusLine(p.id); line != "" {
		if exp := u.StatusExpires; exp > 0 {
			line += sty.Faint.Render(" · until " + clock(time.Unix(exp, 0), m.cfg.TimeFormat))
		}
		field("Status", line)
	}
	switch active, ok := m.presence[p.id]; {
	case u.Deleted:
		rows = append(rows, " "+sty.Err.Render("This account is deactivated"))
	case ok && p.id != m.client.SelfID:
		field("Presence", map[bool]string{true: sty.OK.Render("active"), false: sty.Muted.Render("away")}[active])
	}
	field("Local time", localTime(u, clockNow(), m.cfg.TimeFormat))
	field("Email", u.Email)
	field("Phone", u.Phone)
	if m.extUsers[p.id] {
		field("Workspace", "another organisation (Slack Connect)")
	}
	if p.loading {
		rows = append(rows, " "+sty.Faint.Render("loading…"))
	}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, inner, "…")
	}
	right := ""
	if u.Name != "" {
		right = "@" + u.Name
	}
	return modalBox(w, "Profile", right, rows)
}

func localTime(u backend.User, now time.Time, layout string) string {
	if u.TZ == "" && u.TZOffset == 0 {
		return ""
	}
	loc, err := time.LoadLocation(u.TZ)
	if err != nil || u.TZ == "" {
		loc = time.FixedZone(u.TZLabel, u.TZOffset)
	}
	there := now.In(loc)
	s := there.Format(layout)
	if u.TZLabel != "" {
		s += "  " + sty.Muted.Render(u.TZLabel)
	}
	_, ours := now.Zone()
	_, theirs := there.Zone()
	if d := time.Duration(theirs-ours) * time.Second; d != 0 {
		sign := "+"
		if d < 0 {
			d, sign = -d, "-"
		}
		gap := fmt.Sprintf("%dh", int(d.Hours()))
		if mins := int(d.Minutes()) % 60; mins != 0 {
			gap = fmt.Sprintf("%dh%02dm", int(d.Hours()), mins)
		}
		s += "  " + sty.Faint.Render(sign+gap+" vs you")
	}
	return s
}
