package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/config"
)

// sideItem is a sidebar row: a conversation (id set), a section header
// (header set), or a blank spacer between sections.
type sideItem struct {
	header string
	id     string
	spacer bool
	hidden int // for a folded header: how many conversations it hides
}

func (it sideItem) key() string {
	if it.header != "" {
		return "section:" + it.header
	}
	return it.id
}

// selectable is true for rows the cursor can stop on.
func (it sideItem) selectable() bool { return !it.spacer }

type section struct {
	name string
	ids  []string
}

// sections groups every conversation: custom sections from the config
// first, then Channels, Direct messages and External (Slack Connect).
func (m *Model) sections() []section {
	var out []section
	used := map[string]bool{}
	for _, sec := range m.cfg.Sections {
		var ids []string
		for _, ref := range sec.Items {
			if id := m.resolveRef(ref); id != "" && !used[id] {
				used[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			out = append(out, section{sec.Name, ids})
		}
	}

	var chans, dms, ext []*backend.Conversation
	for id, c := range m.convs {
		if used[id] {
			continue
		}
		if c.IsDM() {
			if c.Kind == backend.KindDM && m.users[c.UserID].Deleted {
				continue
			}
			if c.InfoLoaded && c.Latest == "" { // never had a message
				continue
			}
		}
		switch {
		case m.isExternal(c):
			ext = append(ext, c)
		case c.IsDM():
			dms = append(dms, c)
		default:
			chans = append(chans, c)
		}
	}
	slices.SortFunc(chans, func(a, b *backend.Conversation) int { return strings.Compare(a.Name, b.Name) })
	// DMs by recent activity, like Slack: one-to-one first, then groups.
	byGroupThenRecent := func(a, b *backend.Conversation) int {
		if ag, bg := a.Kind == backend.KindGroup, b.Kind == backend.KindGroup; ag != bg {
			if ag {
				return 1
			}
			return -1
		}
		return m.byRecent(a, b)
	}
	slices.SortFunc(dms, byGroupThenRecent)
	slices.SortFunc(ext, m.byRecent)
	ids := func(cs []*backend.Conversation) []string {
		out := make([]string, len(cs))
		for i, c := range cs {
			out[i] = c.ID
		}
		return out
	}
	if len(chans) > 0 {
		out = append(out, section{"Channels", ids(chans)})
	}
	if len(dms) > 0 {
		out = append(out, section{"Direct messages", ids(dms)})
	}
	if len(ext) > 0 {
		out = append(out, section{"External", ids(ext)})
	}
	return out
}

// byRecent orders conversations newest activity first.
func (m *Model) byRecent(a, b *backend.Conversation) int {
	if a.Latest != b.Latest {
		if backend.TSLess(a.Latest, b.Latest) {
			return 1
		}
		return -1
	}
	return strings.Compare(m.convLabel(a), m.convLabel(b))
}

// isExternal is true for Slack Connect: conversations shared with another
// organisation, and DMs with someone from one.
func (m *Model) isExternal(c *backend.Conversation) bool {
	if c.External {
		return true
	}
	if c.Kind != backend.KindDM {
		return false
	}
	return m.extUsers[c.UserID]
}

func (m *Model) rebuildSidebar() {
	keep := ""
	if m.sideCur < len(m.side) {
		keep = m.side[m.sideCur].key()
	}

	items := []sideItem{{id: unreadsID}}
	for _, sec := range m.sections() {
		items = append(items, sideItem{spacer: true})
		head := sideItem{header: sec.name}
		var rows []sideItem
		for _, id := range sec.ids {
			// A folded section still shows what needs attention: its
			// unread conversations, and the open one.
			if m.folded[sec.name] {
				if c := m.convs[id]; c == nil || (!c.Unread() && id != m.current) {
					head.hidden++
					continue
				}
			}
			rows = append(rows, sideItem{id: id})
		}
		items = append(items, head)
		items = append(items, rows...)
	}

	m.side = items
	m.sideCur = 0
	for i, it := range items {
		if keep != "" && it.key() == keep {
			m.sideCur = i
		}
	}
}

// toggleFold folds or unfolds the section under the cursor (or the one
// the cursor's conversation is in) and remembers it.
func (m *Model) toggleFold() tea.Cmd {
	name := ""
	for i := m.sideCur; i >= 0 && i < len(m.side); i-- {
		if m.side[i].header != "" {
			name = m.side[i].header
			break
		}
	}
	if name == "" {
		return nil
	}
	m.folded[name] = !m.folded[name]
	m.rebuildSidebar()
	m.sideFindKey("section:" + name)

	st := config.LoadState()
	st.Folded = st.Folded[:0]
	for n, f := range m.folded {
		if f {
			st.Folded = append(st.Folded, n)
		}
	}
	slices.Sort(st.Folded)
	if err := config.SaveState(st); err != nil {
		return m.setStatus("saving fold state: "+err.Error(), true)
	}
	return nil
}

func (m *Model) sideFindKey(key string) {
	for i, it := range m.side {
		if it.key() == key {
			m.sideCur = i
			return
		}
	}
}

func (m *Model) unreadCount() int {
	n := 0
	for _, c := range m.convs {
		if c.Unread() {
			n++
		}
	}
	return n
}

func (m *Model) renderSidebar(w, h int) string {
	inner, bodyH := w-2, h-2
	focused := m.focus == fSidebar

	// Keep the cursor in view.
	if m.sideCur < m.sideTop {
		m.sideTop = m.sideCur
	}
	if m.sideCur >= m.sideTop+bodyH {
		m.sideTop = m.sideCur - bodyH + 1
	}
	m.sideTop = max(0, min(m.sideTop, len(m.side)-bodyH))

	secs := m.sections()
	var lines []string
	for i := m.sideTop; i < len(m.side) && i < m.sideTop+bodyH; i++ {
		r := m.sideCells(m.side[i], secs)
		// One cell clear of the frame's right edge.
		lines = append(lines, flagLine(r.flag, r.layout(inner-2)+" ", inner, i == m.sideCur, focused))
	}
	handle := ""
	if u, ok := m.users[m.client.SelfID]; ok && u.Name != "" {
		handle = sty.Faint.Render("@" + u.Name)
	}
	return RenderFrame(strings.Join(lines, "\n"), w, h, frameTitle(m.client.TeamName, focused), handle, focused)
}

// sideCells is a sidebar row cut into its fixed columns (DESIGN.md §6):
// [bar 1][icon 2][name … flex][status 2][dot 1][count].
type sideCells struct {
	flag, icon     string // flag is the bar column (blank), icon one glyph (slot adds the space)
	name, nick     string // already styled; nick follows the name in faint
	status         string // a 2-cell emoji, or ""
	dot            string // the unread marker: red ● for mentions, faint • otherwise
	count          string // styled, right-aligned
	header, spacer bool
}

// layout lays the cells out in width cells: the marker and count flush
// right, the status emoji before them, and the name taking whatever is
// left, all of the row when there is nothing else.
func (r sideCells) layout(width int) string {
	if r.spacer {
		return ""
	}
	var right []string
	if r.status != "" && !r.header {
		right = append(right, r.status)
	}
	if r.dot != "" {
		right = append(right, r.dot)
	}
	if r.count != "" {
		right = append(right, r.count)
	}
	tail := strings.Join(right, " ")
	if tail != "" {
		tail = " " + tail
	}
	room := width - 2 - ansi.StringWidth(tail)
	name := r.name
	if r.nick != "" {
		name += " " + r.nick
	}
	name = ansi.Truncate(name, max(room, 0), "…")
	pad := max(0, room-ansi.StringWidth(name))
	return slot(r.icon) + name + strings.Repeat(" ", pad) + tail
}

// Unread markers, at the right before the count: a red ● for mentions and
// DM unreads, a small faint • for other unread conversations.
func dotMention() string { return sty.Err.Render("●") }

func dotUnread() string { return sty.Faint.Render("•") }

func (m *Model) sideCells(it sideItem, secs []section) sideCells {
	if it.spacer {
		return sideCells{spacer: true}
	}
	if it.header != "" {
		return m.headerCells(it.header, secs)
	}
	if it.id == unreadsID {
		n := m.unreadCount()
		r := sideCells{flag: " ", icon: sty.Muted.Render(ico.Unreads), name: sty.Muted.Render("Unreads")}
		if n > 0 {
			r.icon, r.name = sty.Unread.Render(ico.Unreads), sty.Unread.Render("Unreads")
			r.count = sty.Muted.Render(fmt.Sprint(n))
		}
		return r
	}
	c := m.convs[it.id]
	if c == nil {
		return sideCells{spacer: true}
	}

	label, nick := m.convLabel(c), ""
	if c.Kind == backend.KindDM && c.UserID != m.client.SelfID {
		if u, ok := m.users[c.UserID]; ok && u.RealName != "" {
			label = u.RealName
			if u.DisplayName != "" && u.DisplayName != u.RealName {
				nick = u.DisplayName
			}
		}
	}
	self := c.Kind == backend.KindDM && c.UserID == m.client.SelfID
	if self {
		label = strings.TrimSuffix(label, " (you)")
		nick = "(you)"
	}

	style := sty.Muted
	if it.id == m.current {
		style = sty.Normal
	}
	var r sideCells
	switch {
	case c.Mentions > 0 || (c.IsDM() && c.Unread()):
		style, r.flag, r.dot = sty.Unread, " ", dotMention()
		if c.Mentions > 0 {
			r.count = sty.Err.Render(fmt.Sprint(c.Mentions))
		}
	case c.Unread():
		style, r.flag, r.dot = sty.Unread, " ", dotUnread()
	default:
		r.flag = " "
	}
	r.name = style.Render(label)
	if nick != "" {
		r.nick = sty.Faint.Render(nick)
	}

	switch c.Kind {
	case backend.KindChannel:
		r.icon = style.Render(ico.Channel)
	case backend.KindPrivate:
		r.icon = style.Render(ico.Private)
	case backend.KindGroup:
		r.icon = style.Render(ico.Group)
	case backend.KindDM:
		// Presence is the icon itself: solid in ok when active, outline
		// in faint when away.
		if m.isActive(c.UserID) {
			r.icon = sty.OK.Render(ico.UserActive)
		} else {
			r.icon = sty.Faint.Render(ico.UserAway)
		}
		// The status emoji is drawn as is: a custom one is an image whose
		// placeholder cells must keep their own foreground colour.
		r.status = m.statusEmoji(c.UserID)
	}
	return r
}

// headerCells is a section header: ▾/▸, the section's icon and its name
// in caps. A folded one still says what is inside: mentions with a red
// dot and count, other unreads with a faint dot.
func (m *Model) headerCells(name string, secs []section) sideCells {
	arrow := ico.Unfolded
	if m.folded[name] {
		arrow = ico.Folded
	}
	// The arrow sits in the icon column, the section's own icon before
	// its name.
	title := sty.Header.Render(strings.ToUpper(name))
	if icon := m.sectionIcon(name); icon != "" {
		title = icon + " " + title
	}
	r := sideCells{header: true, flag: " ", icon: sty.Header.Render(arrow), name: title}
	mentions, unread := 0, 0
	for _, sec := range secs {
		if sec.name != name {
			continue
		}
		for _, id := range sec.ids {
			c := m.convs[id]
			if c == nil {
				continue
			}
			switch {
			case c.Mentions > 0 || (c.IsDM() && c.Unread()):
				mentions += max(c.Mentions, 1)
			case c.Unread():
				unread++
			}
		}
	}
	if m.folded[name] {
		switch {
		case mentions > 0:
			r.dot, r.count = dotMention(), sty.Err.Render(fmt.Sprint(mentions))
		case unread > 0:
			r.dot = dotUnread()
		}
	}
	return r
}
