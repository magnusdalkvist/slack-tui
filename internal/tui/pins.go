package tui

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// pinsState is `P`: a conversation's bookmarks, then its pinned messages.
// It takes over the main pane like search.
type pinsState struct {
	conv      string
	bookmarks []backend.Bookmark
	pins      []backend.Pin
	loading   bool
	errs      []string // bookmarks or pins that failed; the other still shows
	sel, top  int
}

type pinsMsg struct {
	conv      string
	bookmarks []backend.Bookmark
	pins      []backend.Pin
	errs      []string
}

// bookmarkEdit is the bookmark `e` is changing.
type bookmarkEdit struct {
	conv string
	b    backend.Bookmark
}

type bookmarkedMsg struct {
	conv, done string
	err        error
}

type pinnedMsg struct {
	conv, ts string
	added    bool
	err      error
}

func (p *pinsState) count() int { return len(p.bookmarks) + len(p.pins) }

func (m *Model) openPins() tea.Cmd {
	if m.current == unreadsID || m.convs[m.current] == nil {
		return m.setStatus("open a conversation first", false)
	}
	m.closeSearch()
	m.shared, m.chans, m.members = nil, nil, nil
	m.pins = &pinsState{conv: m.current, loading: true}
	return m.loadPins(m.current)
}

func (m *Model) loadPins(conv string) tea.Cmd {
	return func() tea.Msg {
		out := pinsMsg{conv: conv}
		var bErr, pErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); out.bookmarks, bErr = m.client.Bookmarks(m.ctx, conv) }()
		go func() { defer wg.Done(); out.pins, pErr = m.client.Pins(m.ctx, conv) }()
		wg.Wait()
		for _, err := range []error{bErr, pErr} {
			if err != nil {
				out.errs = append(out.errs, err.Error())
			}
		}
		return out
	}
}

func (m *Model) pinsResult(msg pinsMsg) {
	p := m.pins
	if p == nil || p.conv != msg.conv {
		return
	}
	p.loading = false
	p.bookmarks, p.pins, p.errs = msg.bookmarks, msg.pins, msg.errs
	p.sel = min(p.sel, max(p.count()-1, 0))
}

func (m *Model) pinsKey(k tea.KeyMsg) tea.Cmd {
	p := m.pins
	n := p.count()
	switch k.String() {
	case "esc", "P":
		m.pins = nil
		return nil
	case "q":
		return m.quit()
	case "/":
		m.pins = nil
		return m.openSearch()
	case "ctrl+k":
		m.pins = nil
		m.switcher = m.newSwitcher()
		return nil
	case "j", "down":
		p.sel++
	case "k", "up":
		p.sel--
	case "g", "home":
		p.sel = 0
	case "G", "end":
		p.sel = n - 1
	case "enter", "l", "right":
		if n == 0 {
			return nil
		}
		if b, pin := p.at(p.sel); b != nil {
			return m.openURL(b.Link)
		} else if pin.TS == "" { // a file pinned on its own
			return m.openURL(pin.Permalink)
		} else {
			m.pins = nil
			return m.goToMessage(pin.Channel, pin.Message)
		}
	case "o":
		if n == 0 {
			return nil
		}
		if b, pin := p.at(p.sel); b != nil {
			return m.openURL(b.Link)
		} else {
			return tea.Batch(m.openURL(pin.Permalink), m.setStatus("opening in Slack…", false))
		}
	case "p":
		if _, pin := p.at(p.sel); pin != nil && pin.TS != "" {
			return m.togglePin(pin.Channel, pin.Message)
		}
	case "a":
		return m.bookmarkCmd("bookmark ", nil)
	case "e":
		if b, _ := p.at(p.sel); b != nil {
			return m.bookmarkCmd("bookmark "+b.Link+" "+b.Title, &bookmarkEdit{conv: p.conv, b: *b})
		}
	case "d", "D", "x":
		if b, _ := p.at(p.sel); b != nil {
			return m.askRemoveBookmark(p.conv, *b)
		}
	}
	p.sel = min(max(p.sel, 0), max(n-1, 0))
	return nil
}

// at returns the bookmark or the pin at row i; both nil when the list is
// empty (or still loading).
func (p *pinsState) at(i int) (*backend.Bookmark, *backend.Pin) {
	if p.count() == 0 {
		return nil, nil
	}
	i = min(max(i, 0), p.count()-1)
	if i < len(p.bookmarks) {
		return &p.bookmarks[i], nil
	}
	return nil, &p.pins[i-len(p.bookmarks)]
}

// togglePin pins or unpins a message. Like reactions, the change shows at
// once and a failed call puts it back.
func (m *Model) togglePin(conv string, msg backend.Message) tea.Cmd {
	add := !msg.Pinned
	m.setPinned(conv, msg.TS, add)
	return func() tea.Msg {
		return pinnedMsg{conv: conv, ts: msg.TS, added: add, err: m.client.Pin(m.ctx, conv, msg.TS, add)}
	}
}

// setPinned marks a message pinned or not wherever it is loaded, and keeps
// an open pins list in step.
func (m *Model) setPinned(conv, ts string, pinned bool) {
	if ts == "" {
		return
	}
	m.touch(conv)
	mark := func(msgs []backend.Message) {
		if i := msgIndex(msgs, ts); i >= 0 {
			msgs[i].Pinned = pinned
		}
	}
	if l, ok := m.lists[conv]; ok {
		mark(l.msgs)
	}
	if t := m.thread; t != nil && t.conv == conv {
		mark(t.list.msgs)
	}
	if u, ok := m.unreadMsgs[conv]; ok {
		mark(u)
	}
	if p := m.pins; p != nil && p.conv == conv && !pinned {
		for i := range p.pins {
			if p.pins[i].TS == ts {
				p.pins = append(p.pins[:i], p.pins[i+1:]...)
				p.sel = min(p.sel, max(p.count()-1, 0))
				break
			}
		}
	}
}

// pinEvent applies a pin made anywhere; an open list of that conversation
// is fetched again, since the event doesn't carry the whole message.
func (m *Model) pinEvent(ev backend.EvPin) tea.Cmd {
	m.setPinned(ev.Channel, ev.TS, ev.Added)
	if p := m.pins; p != nil && p.conv == ev.Channel && (ev.Added || ev.TS == "") {
		return m.loadPins(ev.Channel)
	}
	return nil
}

// renderPins is the Pins & bookmarks pane: BOOKMARKS, then PINNED.
func (m *Model) renderPins(w, h int) string {
	p := m.pins
	lw := m.paneListW(w, true)
	spec := paneSpec{title: frameTitle("Pins & bookmarks", true), top: &p.top}
	if c := m.cmd; c != nil && c.inPane {
		// Adding or changing a bookmark happens in the composer frame.
		c.input.Width = max(10, lw-4)
		spec.input, spec.inputBorder = " "+sty.Faint.Render(":")+c.input.View(), sty.OK
		spec.inputLabel = sty.OK.Bold(true).Render(map[bool]string{true: "editing bookmark", false: "new bookmark"}[c.bookmark != nil])
	}
	if c := m.convs[p.conv]; c != nil {
		spec.right = sty.Faint.Render("in " + m.convTitle(c))
	}
	for _, e := range p.errs {
		spec.notes = append(spec.notes, " "+sty.Err.Render(e))
	}
	spec.empty = "nothing pinned or bookmarked"
	if p.loading {
		spec.empty = "loading…"
	}
	if p.count() == 0 {
		return m.renderPane(spec, w, h)
	}

	sel := min(max(p.sel, 0), p.count()-1)
	spec.fromTop = sel == 0 // keep the first heading in view
	add := func(i int, block []string) {
		if i == sel {
			spec.selStart = len(spec.lines)
		}
		spec.lines = append(spec.lines, blockRows(block, lw, i == sel, true)...)
		if i == sel {
			spec.selEnd = len(spec.lines)
		}
		spec.lines = append(spec.lines, "")
	}
	if len(p.bookmarks) > 0 {
		spec.lines = append(spec.lines, group("BOOKMARKS", fmt.Sprint(len(p.bookmarks))))
	}
	for i, b := range p.bookmarks {
		label := b.Title
		if label == "" {
			label = b.Link
		}
		icon := sty.Faint.Render(ico.Bookmark) + " "
		if name := strings.Trim(b.Emoji, ":"); name != "" {
			icon = m.emojiChip(name) + " "
		}
		add(i, []string{icon + sty.Normal.Render(label), "  " + sty.Md.Link.Render(b.Link)})
	}
	if len(p.pins) > 0 {
		spec.lines = append(spec.lines, group("PINNED", fmt.Sprint(len(p.pins))))
	}
	for i, pin := range p.pins {
		var block []string
		if pin.TS == "" {
			for _, f := range pin.Files {
				block = append(block, fileLine(f, f.Size))
			}
		} else {
			line := sty.Sender(senderKey(pin.Message)).Render(m.senderName(pin.Message)) + " " + sty.Faint.Render(relTime(pin.Time(), m.cfg.TimeFormat))
			if pin.IsReply() {
				line += "  " + sty.Thread.Render("in thread")
			}
			block = append(block, sty.Warn.Render(ico.Pinned)+" "+line)
			text := strings.ReplaceAll(render.Mrkdwn(pin.AllText(), m, sty.Md), "\n", " ")
			if strings.TrimSpace(pin.AllText()) == "" && len(pin.Files) > 0 {
				text = fileLine(pin.Files[0], pin.Files[0].Size)
			}
			wrapped := wrap(text, lw-4)
			if len(wrapped) > 3 {
				wrapped = append(wrapped[:3:3], sty.Faint.Render("…"))
			}
			block = append(block, prefixAll(wrapped, "  ")...)
		}
		add(len(p.bookmarks)+i, block)
	}
	spec.preview = func(w, h int) string {
		if sel < len(p.bookmarks) {
			b := p.bookmarks[sel]
			return strings.Join([]string{sty.Title.Render(b.Title), sty.Md.Link.Render(b.Link)}, "\n")
		}
		pin := p.pins[sel-len(p.bookmarks)]
		return m.previewMessage(m.messageLines(pin.Message, w-1, true, true, false), w, h)
	}
	return m.renderPane(spec, w, h)
}

// bookmarkCmd opens the command line with a :bookmark to finish.
func (m *Model) bookmarkCmd(line string, edit *bookmarkEdit) tea.Cmd {
	cmd := m.openCmd()
	m.cmd.input.SetValue(line)
	m.cmd.input.CursorEnd()
	m.cmd.bookmark = edit
	m.cmd.inPane = m.pins != nil
	return cmd
}

// cmdBookmark adds a link to the conversation's bookmarks, or changes the
// one `e` picked: ":bookmark example.com/runbook Runbook". Without a title
// the link is the title.
func (m *Model) cmdBookmark(args string) tea.Cmd {
	edit := m.bmEdit
	conv := m.current
	if edit != nil {
		conv = edit.conv
	}
	if conv == unreadsID || m.convs[conv] == nil {
		return m.setStatus("open a conversation first", false)
	}
	link, title, _ := strings.Cut(strings.TrimSpace(args), " ")
	if link == "" {
		return m.setStatus("usage: :bookmark link [title]", false)
	}
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	if u, err := url.Parse(link); err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return m.setStatus("not a web link: "+link, true)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSuffix(strings.SplitN(link, "://", 2)[1], "/")
	}
	if edit != nil {
		id := edit.b.ID
		return func() tea.Msg {
			return bookmarkedMsg{conv: conv, done: "bookmark changed", err: m.client.EditBookmark(m.ctx, conv, id, title, link)}
		}
	}
	return func() tea.Msg {
		return bookmarkedMsg{conv: conv, done: "bookmarked " + title, err: m.client.AddBookmark(m.ctx, conv, title, link)}
	}
}

func (m *Model) askRemoveBookmark(conv string, b backend.Bookmark) tea.Cmd {
	label := b.Title
	if label == "" {
		label = b.Link
	}
	m.confirm = &confirmState{
		prompt: "Remove the bookmark " + ansi.Truncate(label, 40, "…") + "?", yesLabel: "remove", noLabel: "keep",
		yes: func() tea.Cmd {
			return func() tea.Msg {
				return bookmarkedMsg{conv: conv, done: "bookmark removed", err: m.client.RemoveBookmark(m.ctx, conv, b.ID)}
			}
		},
	}
	return nil
}

// bookmarkedResult reports a change and fetches an open list again.
func (m *Model) bookmarkedResult(msg bookmarkedMsg) tea.Cmd {
	if msg.err != nil {
		return m.setStatus(msg.err.Error(), true)
	}
	var reload tea.Cmd
	if p := m.pins; p != nil && p.conv == msg.conv {
		p.loading = true
		reload = m.loadPins(msg.conv)
	}
	return tea.Batch(m.setResult(msg.done), reload)
}
