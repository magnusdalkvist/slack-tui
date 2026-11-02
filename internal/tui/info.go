package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// A message's details: `K` on a message, or `:info`. When it was sent and
// edited, who reacted with what, its thread and files, and its link.
// Slack's API has no read receipts and no edit history, so those aren't here.

type infoState struct {
	conv    string
	msg     backend.Message // as it was when opened; live copies win
	sel     int             // into infoItems
	loading bool            // fetching the full reaction lists
}

// infoItem is a row of the pane that enter acts on.
type infoItem struct {
	kind  int // infoThread, infoReaction, infoFile
	index int // into the reactions or files
}

const (
	infoThread = iota
	infoReaction
	infoFile
)

type infoReactionsMsg struct {
	conv, ts  string
	reactions []backend.Reaction
	err       error
}

// openInfo shows the message's details. History cuts the people of
// popular reactions short, so those are fetched in full.
func (m *Model) openInfo(conv string, msg backend.Message) tea.Cmd {
	m.info = &infoState{conv: conv, msg: msg}
	cmds := []tea.Cmd{m.lookupInfoUsers(msg)}
	for _, r := range msg.Reactions {
		if r.Count > len(r.Users) {
			m.info.loading = true
			cmds = append(cmds, func() tea.Msg {
				rs, err := m.client.Reactions(m.ctx, conv, msg.TS)
				return infoReactionsMsg{conv: conv, ts: msg.TS, reactions: rs, err: err}
			})
			break
		}
	}
	return tea.Batch(cmds...)
}

// cmdInfo is `:info`: the selected message of the focused pane.
func (m *Model) cmdInfo() tea.Cmd {
	l, inThread := m.focusedList()
	if l == nil || len(l.msgs) == 0 {
		return m.setStatus("select a message first (K)", false)
	}
	sel := l.sel
	if sel < 0 || sel >= len(l.msgs) {
		sel = len(l.msgs) - 1
	}
	return m.openInfo(m.listConv(inThread), l.msgs[sel])
}

// focusedList is the message list keys go to: the thread's when it has
// focus, else the open conversation's.
func (m *Model) focusedList() (*msgList, bool) {
	if m.thread != nil && m.focus == fThread {
		return &m.thread.list, true
	}
	return m.lists[m.current], false
}

func (m *Model) infoReactionsResult(msg infoReactionsMsg) tea.Cmd {
	i := m.info
	if i != nil && i.conv == msg.conv && i.msg.TS == msg.ts {
		i.loading = false
	}
	if msg.err != nil {
		return m.setStatus("reactions: "+msg.err.Error(), true)
	}
	m.setReactions(msg.conv, msg.ts, msg.reactions)
	if i != nil && i.conv == msg.conv && i.msg.TS == msg.ts {
		i.msg.Reactions = msg.reactions
		return m.lookupInfoUsers(i.msg)
	}
	return nil
}

// setReactions replaces a message's reactions wherever it's shown.
func (m *Model) setReactions(conv, ts string, rs []backend.Reaction) {
	set := func(msgs []backend.Message) {
		if i := msgIndex(msgs, ts); i >= 0 {
			msgs[i].Reactions = rs
		}
	}
	if l := m.lists[conv]; l != nil {
		set(l.msgs)
	}
	if t := m.threadList(conv); t != nil {
		set(t.msgs)
	}
	set(m.unreadMsgs[conv])
	m.touch(conv)
}

// infoMsg is the message as it is now: reactions and replies change while
// the pane is open.
func (m *Model) infoMsg() backend.Message {
	i := m.info
	for _, msgs := range [][]backend.Message{m.listMsgs(m.lists[i.conv]), m.listMsgs(m.threadList(i.conv)), m.unreadMsgs[i.conv]} {
		if k := msgIndex(msgs, i.msg.TS); k >= 0 {
			// Our copy may have the full reaction lists the lists don't.
			msg := msgs[k]
			if len(i.msg.Reactions) > 0 && fullerReactions(i.msg.Reactions, msg.Reactions) {
				msg.Reactions = i.msg.Reactions
			}
			return msg
		}
	}
	return i.msg
}

func (*Model) listMsgs(l *msgList) []backend.Message {
	if l == nil {
		return nil
	}
	return l.msgs
}

// fullerReactions is true when a has the same reactions as b with more of
// the people named.
func fullerReactions(a, b []backend.Reaction) bool {
	if len(a) != len(b) {
		return false
	}
	more := false
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Count != b[i].Count {
			return false
		}
		more = more || len(a[i].Users) > len(b[i].Users)
	}
	return more
}

// lookupInfoUsers fetches the people the pane names that we don't know.
func (m *Model) lookupInfoUsers(msg backend.Message) tea.Cmd {
	if m.userLookups == nil {
		m.userLookups = map[string]bool{}
	}
	var cmds []tea.Cmd
	want := func(id string) {
		if _, known := m.users[id]; known || id == "" || m.userLookups[id] {
			return
		}
		m.userLookups[id] = true
		cmds = append(cmds, m.fetchUser(id))
	}
	for _, r := range msg.Reactions {
		for _, u := range r.Users {
			want(u)
		}
	}
	for _, u := range msg.ReplyUsers {
		want(u)
	}
	return tea.Batch(cmds...)
}

func (m *Model) infoItems(msg backend.Message) []infoItem {
	var items []infoItem
	if msg.ReplyCount > 0 || msg.IsReply() {
		items = append(items, infoItem{kind: infoThread})
	}
	for k := range mergeSkinTones(msg.Reactions) {
		items = append(items, infoItem{kind: infoReaction, index: k})
	}
	for k := range msg.Files {
		items = append(items, infoItem{kind: infoFile, index: k})
	}
	return items
}

func (m *Model) infoKey(k tea.KeyMsg) tea.Cmd {
	i := m.info
	msg := m.infoMsg()
	items := m.infoItems(msg)
	i.sel = min(max(i.sel, 0), max(len(items)-1, 0))
	var it *infoItem
	if len(items) > 0 {
		it = &items[i.sel]
	}
	switch k.String() {
	case "esc", "q", "K":
		m.info = nil
	case "j", "down":
		i.sel = min(i.sel+1, max(len(items)-1, 0))
	case "k", "up":
		i.sel = max(i.sel-1, 0)
	case "g", "home":
		i.sel = 0
	case "G", "end":
		i.sel = max(len(items)-1, 0)
	case "c":
		return m.copyText(m.client.Permalink(i.conv, msg.TS, msg.ThreadTS))
	case "+":
		m.info = nil
		return m.openPicker(i.conv, msg)
	case "o":
		if it != nil && it.kind == infoFile {
			return m.openFile(msg.Files[it.index])
		}
		return tea.Batch(m.openURL(m.client.Permalink(i.conv, msg.TS, msg.ThreadTS)), m.setStatus("opening in Slack…", false))
	case "s":
		if it != nil && it.kind == infoFile {
			return m.saveFiles(msg.Files[it.index : it.index+1])
		}
		return m.saveFiles(msg.Files)
	case "enter":
		if it == nil {
			return nil
		}
		switch it.kind {
		case infoThread:
			m.info = nil
			if msg.IsReply() || i.conv != m.current {
				return m.goToMessage(i.conv, msg)
			}
			return m.openThread(msg)
		case infoReaction:
			// The merged reaction toggles our own skin tone, if we used one.
			r := mergeSkinTones(msg.Reactions)[it.index]
			name := r.Name
			for _, raw := range msg.Reactions {
				if slices.Contains(raw.Users, m.client.SelfID) && render.BaseEmoji(raw.Name) == r.Name {
					name = raw.Name
				}
			}
			cmd := m.toggleReaction(i.conv, msg.TS, name)
			i.msg = m.infoMsg()
			return cmd
		case infoFile:
			return m.openFile(msg.Files[it.index])
		}
	}
	return nil
}

// renderInfo is the Message modal: who and where, the details in a
// 12-cell label column, then reactions with their people and the files.
// Rows enter acts on are selectable; a tall pane scrolls with the selection.
func (m *Model) renderInfo() string {
	i := m.info
	msg := m.infoMsg()
	w := m.modalWidth(72)
	inner := w - 2
	items := m.infoItems(msg)
	i.sel = min(max(i.sel, 0), max(len(items)-1, 0))

	head := " " + sty.Title.Render(m.senderName(msg))
	if msg.User == "" && (msg.BotID != "" || msg.BotName != "") {
		head += " " + sty.AppTag.Render("APP")
	}
	if c := m.convs[i.conv]; c != nil {
		where := m.convLabel(c)
		if !c.IsDM() {
			where = "#" + c.Name
		}
		head += "  " + sty.Faint.Render("in "+where)
	}
	rows := []string{head, modalRule(inner)}
	selRow := -1 // the first row of the selected item
	item := 0
	add := func(lines []string, selectable bool) {
		sel := selectable && item == i.sel && len(items) > 0
		if sel {
			selRow = len(rows)
		}
		for _, l := range lines {
			rows = append(rows, modalRow(l, inner, sel))
		}
		if selectable {
			item++
		}
	}
	field := func(label, value string) string {
		return sty.Faint.Render(fmt.Sprintf("%-12s", label)) + value
	}

	sent := msg.Time()
	add([]string{field("Sent", fullTime(sent, m.cfg.TimeFormat)+sty.Faint.Render(" · "+ago(sent, clockNow())))}, false)
	if msg.Edited {
		v := "yes"
		if msg.EditedTS != "" {
			v = fullTime(backend.TSTime(msg.EditedTS), m.cfg.TimeFormat)
		}
		add([]string{field("Edited", v)}, false)
	}
	if msg.Pinned {
		add([]string{field("Pinned", ico.Pinned+" in this conversation")}, false)
	}
	if msg.SubType == "thread_broadcast" {
		add([]string{field("Thread", "a reply, also sent to the channel")}, false)
	}
	if msg.ReplyCount > 0 || msg.IsReply() {
		v := sty.Thread.Render(ico.Thread + " a reply in a thread")
		if !msg.IsReply() {
			v = sty.Thread.Render(fmt.Sprintf("%s %d %s", ico.Thread, msg.ReplyCount, plural(msg.ReplyCount, "reply", "replies")))
			if msg.LatestReply != "" {
				v += sty.Faint.Render(" · last " + ago(backend.TSTime(msg.LatestReply), clockNow()))
			}
		}
		lines := []string{field("Thread", v)}
		if len(msg.ReplyUsers) > 0 {
			lines = append(lines, m.peopleLines(msg.ReplyUsers, inner-1, 12, "from ")...)
		}
		add(lines, true)
	}
	add([]string{field("Link", sty.Muted.Render(m.client.Permalink(i.conv, msg.TS, msg.ThreadTS)))}, false)

	if rs := mergeSkinTones(msg.Reactions); len(rs) > 0 {
		rows = append(rows, "", modalHeader("Reactions"))
		for _, r := range rs {
			style := sty.Reaction
			if slices.Contains(r.Users, m.client.SelfID) {
				style = sty.ReactSelf
			}
			chip := style.Render(fmt.Sprintf("%s %d", m.emojiChip(r.Name), r.Count))
			lead := chip + " " + sty.Faint.Render(":"+r.Name+":")
			people := m.peopleLines(r.Users, inner-1, 2, "")
			if missing := r.Count - len(r.Users); missing > 0 {
				more := sty.Faint.Render(fmt.Sprintf("and %d more", missing))
				if i.loading {
					more = sty.Faint.Render("loading…")
				}
				people = append(people, "  "+more)
			}
			add(append([]string{lead}, people...), true)
		}
	}
	if len(msg.Files) > 0 {
		rows = append(rows, "", modalHeader("Files"))
		for _, f := range msg.Files {
			name := f.Name
			if f.Title != "" && f.Title != f.Name {
				name = f.Title
			}
			var meta []string
			if f.Mimetype != "" {
				meta = append(meta, f.Mimetype)
			}
			if f.Size > 0 {
				meta = append(meta, fmtSize(f.Size))
			}
			add([]string{rightAlign(fileIcon(f.Mimetype, f.Name)+" "+name, sty.Faint.Render(strings.Join(meta, " · ")), inner)}, true)
		}
	}

	// A tall pane shows the rows around the selection.
	room := max(6, m.height*3/4-2)
	if len(rows) > room {
		top := 0
		if selRow >= 0 {
			top = min(max(0, selRow-room/3), len(rows)-room)
		}
		rows = rows[top : top+room]
	}
	for k, r := range rows {
		rows[k] = ansi.Truncate(r, inner, "…")
	}
	return modalBox(w, "Message", msg.Time().Format(m.cfg.TimeFormat), rows)
}

// peopleLines names the people, wrapped to width with indent cells
// before each line; we come first, as "you".
func (m *Model) peopleLines(ids []string, width, indent int, prefix string) []string {
	var names []string
	self := false
	for _, id := range ids {
		if id == m.client.SelfID {
			self = true
			continue
		}
		n := m.UserName(id)
		if n == "" {
			n = id
		}
		names = append(names, n)
	}
	if self {
		names = append([]string{"you"}, names...)
	}
	if len(names) == 0 {
		return nil
	}
	pad := strings.Repeat(" ", indent)
	var out []string
	for _, l := range wrap(prefix+strings.Join(names, ", "), max(width-indent, 10)) {
		out = append(out, pad+sty.Muted.Render(l))
	}
	return out
}

// fullTime is a time with its date, always: "Mon 2 Nov 2026 14:03".
func fullTime(t time.Time, layout string) string {
	return t.Format("Mon 2 Jan 2006 " + layout)
}
