package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// clockNow is the time views draw against; tests pin it for snapshots.
var clockNow = time.Now

// systemSubtypes are rendered as dim one-liners rather than chat messages.
var systemSubtypes = map[string]bool{
	"channel_join": true, "channel_leave": true, "group_join": true, "group_leave": true,
	"channel_topic": true, "channel_purpose": true, "channel_name": true,
	"group_topic": true, "group_purpose": true, "group_name": true,
	"channel_archive": true, "channel_unarchive": true, "pinned_item": true,
	"bot_add": true, "bot_remove": true,
}

// block is the rendered lines of one message plus where they start.
type block struct {
	lines []string
	start int
}

// renderList draws a message list into width×height, keeping the selected
// message in view. In a thread every reply is shown in full.
func (m *Model) renderList(l *msgList, width, height int, active, inThread bool) string {
	if l == nil || (!l.loaded && len(l.msgs) == 0) {
		return placeholder("loading…", width, height)
	}
	if len(l.msgs) == 0 {
		return placeholder("no messages yet", width, height)
	}

	sel := l.sel
	if sel < 0 || sel >= len(l.msgs) {
		sel = len(l.msgs) - 1
	}

	var all []string
	blocks := make([]block, len(l.msgs))
	gw := m.gutterW()
	if l.hasMore && !inThread {
		hint := "older messages above"
		if l.loading {
			hint = "loading older messages…"
		}
		all = append(all, sty.Faint.Render(center("── "+hint+" ──", width)))
	}
	var prev *backend.Message
	for i := range l.msgs {
		msg := &l.msgs[i]
		var pre []string
		if prev == nil || !sameDay(prev.Time(), msg.Time()) {
			pre = append(pre, dateRule(dayLabel(msg.Time()), width))
		}
		isNew := l.newSince != "" && backend.TSLess(l.newSince, msg.TS) && (prev == nil || !backend.TSLess(l.newSince, prev.TS))
		if isNew {
			pre = append(pre, newRule(width))
		}
		grouped := prev != nil && len(pre) == 0 && !(inThread && i == 1) &&
			senderKey(*prev) == senderKey(*msg) &&
			!systemSubtypes[prev.SubType] && !systemSubtypes[msg.SubType] &&
			msg.Time().Sub(prev.Time()) < 5*time.Minute
		body := m.messageLines(*msg, width-1, !grouped, inThread, i == sel)

		all = append(all, pre...)
		blocks[i].start = len(all)
		all = append(all, m.paintMessage(*msg, body, width, i == sel, active)...)
		blocks[i].lines = body
		prev = msg
	}

	// Scroll: follow the bottom, or keep the selected block on screen. A
	// block taller than the pane keeps wherever j/k scrolled it to.
	total := len(all)
	maxTop := max(0, total-height)
	b := blocks[sel]
	end := b.start + len(b.lines)
	switch {
	case l.sel < 0:
		l.top = maxTop
	case len(b.lines) > height:
		l.top = min(max(l.top, b.start), end-height)
		if sel == 0 && l.top == b.start {
			l.top = 0
		}
	default:
		if b.start < l.top {
			l.top = b.start
		}
		if end > l.top+height {
			l.top = end - height
		}
		if sel == 0 {
			l.top = 0 // show the "load older" hint and date line
		}
	}
	l.top = min(max(l.top, 0), maxTop)
	l.spans = l.spans[:0]
	for _, b := range blocks {
		l.spans = append(l.spans, span{b.start, len(b.lines)})
	}
	l.viewH, l.total = height, total

	view := append([]string(nil), all[l.top:min(total, l.top+height)]...)
	// A selected message taller than the pane keeps its sender line on
	// the top row, saying which lines show, and the bottom row counts the
	// lines still below.
	if height >= 4 && len(view) == height && len(b.lines) > height {
		msg := l.msgs[sel]
		first, last := max(l.top, b.start)-b.start+1, min(l.top+height, end)-b.start
		if b.start < l.top+height && end > l.top {
			head := m.stickyHead(msg, first, last, len(b.lines), width-1)
			view[0] = m.paintMessage(msg, []string{head}, width, true, active)[0]
		}
		if below := end - (l.top + height); below > 0 {
			more := strings.Repeat(" ", gw) + sty.Thread.Render(fmt.Sprintf("%s %d more %s", ico.Unfolded, below+1, plural(below+1, "line", "lines")))
			view[height-1] = m.paintMessage(msg, []string{more}, width, true, active)[0]
		}
	}
	pad := make([]string, 0, height)
	for range height - len(view) {
		pad = append(pad, "")
	}
	return strings.Join(append(pad, view...), "\n")
}

func prefixAll(lines []string, p string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = p + l
	}
	return out
}

// stickyHead stands in for the sender line of a message taller than the
// pane: who wrote it, when, and which of its lines are on screen.
func (m *Model) stickyHead(msg backend.Message, first, last, total, width int) string {
	head := m.gutter(msg) + m.senderLine(msg) +
		sty.Faint.Render(fmt.Sprintf(" · lines %d–%d of %d", first, last, total))
	return ansi.Truncate(head, width, "…")
}

// gutterW is the time gutter: the time plus a space, at least 6 cells.
func (m *Model) gutterW() int {
	return max(6, ansi.StringWidth(time.Date(2006, 1, 2, 23, 59, 0, 0, time.UTC).Format(m.cfg.TimeFormat))+1)
}

// gutter is a message's time, padded to the gutter.
func (m *Model) gutter(msg backend.Message) string {
	t := msg.Time().Format(m.cfg.TimeFormat)
	return sty.Time.Render(t) + strings.Repeat(" ", max(0, m.gutterW()-ansi.StringWidth(t)))
}

// dateRule is "──── Thursday 22 October ────" across the whole list.
func dateRule(label string, width int) string {
	label = " " + label + " "
	side := max(0, width-ansi.StringWidth(label))
	return sty.Faint.Render(strings.Repeat("─", side/2)) + sty.Date.Render(label) + sty.Faint.Render(strings.Repeat("─", side-side/2))
}

// newRule is "──── new ────" in mention across the whole list: where
// the unread messages start.
func newRule(width int) string {
	label := " new "
	side := max(0, width-ansi.StringWidth(label))
	return sty.NewLine.Render(strings.Repeat("─", side/2)) + sty.NewLine.Bold(true).Render(label) +
		sty.NewLine.Render(strings.Repeat("─", side-side/2))
}

// senderLine is the name in its colour, then the handle, an app's tag
// and whether the message is pinned.
func (m *Model) senderLine(msg backend.Message) string {
	line := sty.Sender(senderKey(msg)).Render(m.senderName(msg))
	if msg.User == "" && (msg.BotName != "" || msg.BotID != "") {
		tag := sty.AppTag.Render("APP")
		if ico.Bot != "" {
			tag = sty.Muted.Render(ico.Bot) + " " + tag
		}
		line += " " + tag
	} else if u, ok := m.users[msg.User]; ok && u.Name != "" && u.Name != m.senderName(msg) {
		line += " " + sty.Faint.Render(u.Name)
	}
	if msg.Pinned {
		line += "  " + sty.Warn.Render(ico.Pinned+" pinned")
	}
	return line
}

// mentionsMe is true for messages that mention us, or everyone here.
func (m *Model) mentionsMe(msg backend.Message) bool {
	return strings.Contains(msg.Text, "<@"+m.client.SelfID+">") ||
		strings.Contains(msg.Text, "<!here") || strings.Contains(msg.Text, "<!channel") || strings.Contains(msg.Text, "<!everyone")
}

// paintMessage lays a message's lines into the list: the bar column, then
// the line across the width. Selection wins over the tints for messages
// that mention us (self bar on mentionTint) and pinned ones (warn bar on
// pinTint).
func (m *Model) paintMessage(msg backend.Message, lines []string, width int, selected, focused bool) []string {
	flag, tint := " ", ""
	switch {
	case m.mentionsMe(msg):
		flag, tint = sty.SelfBar(), sty.MentionTintOn()
	case msg.Pinned:
		flag, tint = sty.PinBar(), sty.PinTintOn()
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if selected || tint == "" {
			out[i] = flagLine(flag, l, width, selected, focused)
		} else {
			out[i] = flag + paintBg(l, width-1, tint)
		}
	}
	return out
}

// messageLines is a message without its bar column, width cells wide:
// the time gutter on its first line, the body after it.
func (m *Model) messageLines(msg backend.Message, width int, header, inThread, sel bool) []string {
	gw := m.gutterW()
	bw := width - gw
	blank := strings.Repeat(" ", gw)
	var lines []string
	if systemSubtypes[msg.SubType] {
		text := render.Mrkdwn(msg.Text, m, sty.Md)
		for i, l := range wrap(sty.System.Render("→ ")+sty.System.Render(ansi.Strip(text)), bw) {
			lines = append(lines, pick(i == 0, m.gutter(msg), blank)+l)
		}
		return lines
	}
	if header {
		lines = append(lines, m.senderLine(msg))
	}
	text := render.Mrkdwn(msg.Text, m, sty.Md)
	if msg.Edited {
		text += " " + sty.Faint.Render("(edited)")
	}
	if msg.Pinned && !header {
		text += " " + sty.Warn.Render(ico.Pinned+" pinned")
	}
	if strings.TrimSpace(msg.Text) != "" || msg.Edited || (msg.Pinned && !header) {
		codeOn := sgrPrefix(sty.Md.CodeBlock)
		for _, l := range wrap(text, bw) {
			// Code blocks fill the body column.
			if codeOn != "" && strings.HasPrefix(l, codeOn) {
				l = paintBg(l, bw, sty.RaisedBgOn())
			}
			lines = append(lines, l)
		}
	}
	for _, f := range msg.Files {
		lines = append(lines, m.imageLines(f, bw, sel)...)
	}
	if len(msg.Reactions) > 0 {
		var chips []string
		for _, r := range mergeSkinTones(msg.Reactions) {
			chip := fmt.Sprintf("%s %d", m.emojiChip(r.Name), r.Count)
			style := sty.Reaction
			if slices.Contains(r.Users, m.client.SelfID) {
				style = sty.ReactSelf
			}
			chips = append(chips, style.Render(chip))
		}
		lines = append(lines, wrap(strings.Join(chips, " "), bw)...)
	}
	if !inThread && msg.ReplyCount > 0 {
		s := fmt.Sprintf("%s %d %s", ico.Thread, msg.ReplyCount, plural(msg.ReplyCount, "reply", "replies"))
		if msg.LatestReply != "" {
			s += " · last " + ago(backend.TSTime(msg.LatestReply), clockNow())
		}
		lines = append(lines, sty.Thread.Render(s))
	}
	if len(lines) == 0 {
		lines = append(lines, sty.Faint.Render("(empty message)"))
	}
	for i := range lines {
		lines[i] = pick(i == 0, m.gutter(msg), blank) + lines[i]
	}
	return lines
}

func pick(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// ago is a short relative time: "just now", "5m ago", "2h ago", "3d ago".
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func wrap(s string, width int) []string {
	if width < 4 {
		width = 4
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, hang(line, width)...)
	}
	return linksPerLine(out)
}

// reItem is the start of a list item: "• ", "  ◦ ", "- ", "12. ".
var reItem = regexp.MustCompile(`^ *(?:[•◦▪‣-]|\d{1,3}[.)]) +`)

// hang wraps one line; a list item's later lines line up under its text,
// not under the bullet, as in Slack.
func hang(line string, width int) []string {
	n := len(reItem.FindString(line)) // the bullet is plain text, so bytes line up
	pre := line[:n]
	w := ansi.StringWidth(pre)
	if n == 0 || width-w < 10 {
		return strings.Split(ansi.Wrap(line, width, ""), "\n")
	}
	lines := strings.Split(ansi.Wrap(line[n:], width-w, ""), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = pre + lines[i]
		} else {
			lines[i] = strings.Repeat(" ", w) + lines[i]
		}
	}
	return lines
}

var reEsc = regexp.MustCompile("\x1b\\[[0-9;:]*m|\x1b]8;[^;\x1b]*;([^\x1b]*)\x1b\\\\")

// linksPerLine makes every wrapped line self-contained: colours, underline
// and hyperlinks still open at the end of a line are closed there and
// reopened on the next. Wrapping alone leaves them open across the newline,
// and the terminal then underlines (or links) the rest of that row and
// whatever is drawn next to it, like the neighbouring pane.
func linksPerLine(lines []string) []string {
	carry := ""
	for i, l := range lines {
		l = carry + l
		var sgr []string
		link := ""
		for _, m := range reEsc.FindAllStringSubmatch(l, -1) {
			switch seq := m[0]; {
			case strings.HasPrefix(seq, "\x1b]8;"):
				link = ""
				if m[1] != "" {
					link = seq
				}
			case seq == "\x1b[m" || seq == "\x1b[0m":
				sgr = sgr[:0]
			default:
				sgr = append(sgr, seq)
			}
		}
		carry = strings.Join(sgr, "") + link
		if len(sgr) > 0 {
			l += "\x1b[0m"
		}
		if link != "" {
			l += "\x1b]8;;\x1b\\"
		}
		lines[i] = l
	}
	return lines
}

func placeholder(s string, width, height int) string {
	lines := make([]string, height)
	if height > 0 {
		lines[height/2] = sty.Muted.Render(center(s, width))
	}
	return strings.Join(lines, "\n")
}

func center(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return ansi.Truncate(s, width, "…")
	}
	return strings.Repeat(" ", (width-w)/2) + s
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func dayLabel(t time.Time) string {
	now := clockNow()
	switch {
	case sameDay(t, now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Year() == now.Year():
		return t.Format("Monday 2 January")
	default:
		return t.Format("Monday 2 January 2006")
	}
}

func relTime(t time.Time, layout string) string {
	if sameDay(t, clockNow()) {
		return t.Format(layout)
	}
	return t.Format("2 Jan " + layout)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// mergeSkinTones combines reactions that differ only by skin tone, since
// skin tones are shown as the base emoji: 🙏 2 and 🙏 1 become 🙏 3.
func mergeSkinTones(rs []backend.Reaction) []backend.Reaction {
	var out []backend.Reaction
	at := map[string]int{}
	for _, r := range rs {
		base := render.BaseEmoji(r.Name)
		if i, ok := at[base]; ok {
			out[i].Count += r.Count
			out[i].Users = append(out[i].Users, r.Users...)
			continue
		}
		at[base] = len(out)
		out = append(out, backend.Reaction{Name: base, Count: r.Count, Users: append([]string(nil), r.Users...)})
	}
	return out
}
