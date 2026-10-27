package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

func (m *Model) View() string {
	m.viewSeq++ // images drawn below record it; only those animate
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.bootErr != nil {
		return m.renderFatal(m.height-1) + "\n" + m.renderStatus()
	}
	if !m.booted {
		return m.renderLoading(m.height-1) + "\n" + m.loadingBar()
	}

	if m.viewer != nil {
		return m.renderViewer() + "\n" + m.renderStatus()
	}

	h := m.height - 1
	var body string
	if m.layout() == layoutStack {
		// One frame fills the window: the conversations, the chat or the
		// thread, whichever has focus; panes take the chat's level.
		switch main := m.renderMain(m.width, h, true); {
		case main != "":
			body = fit(main, m.width, h)
		default:
			body = m.renderSidebar(m.width, h)
		}
	} else {
		sw := min(m.cfg.SidebarWidth, m.width/3)
		mainW := m.width - sw - 1
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(sw, h), gapCol(h), fit(m.renderMain(mainW, h, false), mainW, h))
	}
	if box := m.renderModal(); box != "" {
		body = m.overlayModal(body, box)
	}
	if m.cmd != nil && !m.cmd.inPane {
		return m.overlayCmdCompletion(body) + "\n" + m.renderCmdLine()
	}
	return body + "\n" + m.renderStatus()
}

// renderModal is the open modal's box, or "".
func (m *Model) renderModal() string {
	switch {
	case m.switcher != nil:
		return m.renderSwitcher()
	case m.picker != nil:
		return m.renderPicker()
	case m.opener != nil:
		return m.renderOpener()
	case m.profile != nil:
		return m.renderProfile()
	}
	return ""
}

// fit forces s to exactly w×h cells so panes line up.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		l = ansi.Truncate(l, w, "")
		if pad := w - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// gapCol is the blank column between frames.
func gapCol(h int) string {
	return strings.TrimSuffix(strings.Repeat(" \n", h), "\n")
}

func rule(w int, active bool) string {
	if active {
		return sty.Bar.Render(strings.Repeat("─", w))
	}
	return sty.Rule.Render(strings.Repeat("─", w))
}

func (m *Model) convTitle(c *backend.Conversation) string {
	switch c.Kind {
	case backend.KindChannel:
		return ico.Channel + " " + c.Name
	case backend.KindPrivate:
		return ico.Private + " " + c.Name
	default:
		return m.convLabel(c)
	}
}

func (m *Model) renderConvPane(w, h int) string {
	c := m.convs[m.current]
	if c == nil {
		return placeholder("conversation not found", w, h)
	}
	focused := m.focus == fMessages
	var ctx []string
	if c.Kind == backend.KindDM {
		if m.isActive(c.UserID) {
			ctx = append(ctx, sty.OK.Render(ico.UserActive))
		}
		if line := m.statusLine(c.UserID); line != "" {
			if exp := m.users[c.UserID].StatusExpires; exp > 0 {
				line += sty.Faint.Render(" · until " + clock(time.Unix(exp, 0), m.cfg.TimeFormat))
			}
			ctx = append(ctx, line)
		}
	}
	if c.Topic != "" {
		ctx = append(ctx, sty.Topic.Render(strings.ReplaceAll(render.Mrkdwn(c.Topic, m, sty.Md), "\n", " ")))
	}
	right := ""
	if n := m.scheduledIn(c.ID); n > 0 {
		right = sty.Faint.Render(fmt.Sprintf("%s %d scheduled", ico.Scheduled, n))
	}
	title := m.convTitle(c)
	if m.stacked() {
		// The level above is the conversation list: ‹ and a breadcrumb.
		if right != "" {
			ctx = append(ctx, right)
		}
		title, right = "‹ "+title, sty.Faint.Render(m.client.TeamName+" › "+m.convLabel(c))
	}
	isTarget := !(m.focus == fThread && m.thread != nil)
	input := m.renderInput(w, isTarget, false, "Message "+m.convTitle(c))
	paneH := max(3, h-lineCount(input))
	list := m.renderList(m.lists[m.current], w-2, paneH-2, focused, false)
	pane := RenderFrame(list, w, paneH, frameTitle(title, focused, ctx...), right, focused)
	return m.withCompletion(pane, input, w, isTarget)
}

func (m *Model) renderThreadPane(w, h int) string {
	t := m.thread
	focused := m.focus == fThread
	where := ""
	if c := m.convs[t.conv]; c != nil {
		where = sty.Muted.Render("in " + m.convTitle(c))
	}
	input := m.renderInput(w, focused, true, "Reply in thread")
	paneH := max(3, h-lineCount(input))
	list := m.renderList(&t.list, w-2, paneH-2, focused, true)
	title, right := "Thread", ""
	if c := m.convs[t.conv]; c != nil && m.layout() != layoutWide {
		title, right = "‹ Thread", sty.Faint.Render(m.convLabel(c)+" › Thread")
	}
	pane := RenderFrame(list, w, paneH, frameTitle(title, focused, where), right, focused)
	return m.withCompletion(pane, input, w, focused)
}

// withCompletion stacks the pane on the composer, with the completion
// popup laid over the pane's bottom rows so it sits flush on the
// composer's top edge, aligned to its left.
func (m *Model) withCompletion(above, input string, w int, isTarget bool) string {
	if m.complete == nil || !isTarget || !m.insert {
		return above + "\n" + input
	}
	pw := min(w, 56)
	rows := m.renderCompletion(pw)
	lines := strings.Split(above, "\n")
	at := max(0, len(lines)-len(rows))
	for k, r := range rows {
		if i := at + k; i < len(lines) {
			lines[i] = r + ansi.TruncateLeft(lines[i], pw, "")
		}
	}
	return strings.Join(lines, "\n") + "\n" + input
}

// renderInput is the composer: a small frame under the pane it sends to,
// its border following the mode (rule, ok while typing). Its labels sit
// in the top border; the thread's composer says on the right whether the
// reply also goes to the channel. The other pane's composer shows only its
// placeholder.
func (m *Model) renderInput(w int, isTarget, inThread bool, placeholder string) string {
	right := ""
	if inThread {
		right = m.broadcastLabel()
	}
	if !isTarget || !m.insert {
		return frameWith(" "+sty.Faint.Render(placeholder), w, 3, "", right, sty.Rule)
	}
	m.input.SetWidth(w - 4)
	m.fitInput() // the width may have changed, and with it the wrapping
	border := sty.OK
	// Where it goes stays in the border while the placeholder is gone.
	labels := []string{placeholder}
	if m.editing != nil {
		labels = append(labels, "editing")
	}
	if label := m.attachLabel(); label != "" {
		labels = append(labels, label)
	}
	box := indent(m.input.View(), " ")
	if len(m.attach) > 0 {
		box = " " + m.attachChips(w-4) + "\n" + box
	}
	return frameWith(box, w, lineCount(box)+2, border.Bold(true).Render(strings.Join(labels, " · ")), right, border)
}

// broadcastLabel is the thread composer's "☐ also send to #dev", ticked and in
// warn while ctrl+o has it on.
func (m *Model) broadcastLabel() string {
	if m.thread == nil || m.editing != nil {
		return ""
	}
	c := m.convs[m.thread.conv]
	if c == nil {
		return ""
	}
	where := "#" + c.Name
	if c.IsDM() {
		where = m.convLabel(c)
	}
	if m.broadcast {
		return sty.Warn.Render(ico.CheckOn + " also send to " + where)
	}
	return sty.Faint.Render(ico.CheckOff + " also send to " + where)
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func lineCount(s string) int { return strings.Count(s, "\n") + 1 }

// renderUnreads is the Unreads pane: a block per conversation, its last
// messages in the usual message anatomy, three lines each at most.
func (m *Model) renderUnreads(w, h int) string {
	convs := m.unreadConvs()
	focused := m.focus == fMessages
	lw := m.paneListW(w, false)
	u := &m.unreads
	p := paneSpec{title: frameTitle("Unreads", focused), top: &u.top, empty: "All caught up ✓"}
	if len(convs) > 0 {
		p.right = sty.Faint.Render(fmt.Sprintf("%d %s", len(convs), plural(len(convs), "conversation", "conversations")))
	}
	if len(convs) == 0 && m.infoPending > 0 {
		p.empty = "checking for unreads…"
	}
	sel := min(max(u.sel, 0), len(convs)-1)
	for i, c := range convs {
		if i == sel {
			p.selStart = len(p.lines)
		}
		r := m.sideCells(sideItem{id: c.ID}, nil)
		head := r.flag + slot(r.icon) + sty.Unread.Render(m.convLabel(c))
		meta := sty.Faint.Render(relTime(backend.TSTime(c.Latest), m.cfg.TimeFormat))
		if r.count != "" {
			meta = r.count + "  " + meta
		}
		block := []string{rightAlign(head, meta, lw-1)}
		msgs, ok := m.unreadMsgs[c.ID]
		switch {
		case !ok:
			block = append(block, "  "+sty.Faint.Render("loading…"))
		case len(msgs) == 0:
			block = append(block, "  "+sty.Faint.Render("only thread replies or hidden messages"))
		default:
			const show = 5
			if len(msgs) > show {
				block = append(block, "  "+sty.Faint.Render(fmt.Sprintf("… %d earlier", len(msgs)-show)))
				msgs = msgs[len(msgs)-show:]
			}
			for j, msg := range msgs {
				header := j == 0 || senderKey(msgs[j-1]) != senderKey(msg)
				lines := m.messageLines(msg, lw-2, header, false, false)
				if limit := 3 + map[bool]int{true: 1, false: 0}[header]; len(lines) > limit {
					lines = append(lines[:limit-1:limit-1], strings.Repeat(" ", m.gutterW())+sty.Faint.Render("…"))
				}
				block = append(block, prefixAll(lines, " ")...)
			}
		}
		p.lines = append(p.lines, selLines(block, lw, i == sel, focused)...)
		if i == sel {
			p.selEnd = len(p.lines)
		}
		p.lines = append(p.lines, "")
	}
	return m.renderPane(p, w, h)
}
