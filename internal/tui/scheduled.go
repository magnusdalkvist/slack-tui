package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// Scheduled messages: `:later <when>` (ctrl+s in insert mode) sends the
// composer's text later, `:scheduled` lists what is waiting, across all
// conversations, and can cancel it or take it back into the composer.

// schedState is the `:scheduled` pane; its rows are m.upcoming().
type schedState struct {
	loading  bool
	err      string
	sel, top int
}

type scheduledListMsg struct {
	list []backend.Scheduled
	err  error
}

type scheduledMsg struct {
	s        backend.Scheduled
	threadTS string
	text     string // put back in the composer if it fails
	err      error
}

type unscheduledMsg struct {
	s        backend.Scheduled
	toInput  bool // `e`: the text goes back into the composer
	threadTS string
	err      error
}

// Slack refuses times further ahead than this.
const maxSchedule = 120 * 24 * time.Hour

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday, "mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// parseLater reads when to send: "30m", "1h30m", "in 2h", "15:30", "3pm",
// "tomorrow", "tomorrow 8:30", "mon", "friday 14". A day without a time
// means 9:00; a bare hour below 7 on a named day means the afternoon.
func parseLater(args string, now time.Time) (time.Time, error) {
	var words []string
	for _, w := range strings.Fields(strings.ToLower(args)) {
		switch {
		case w == "in" || w == "at":
		case (w == "am" || w == "pm") && len(words) > 0:
			words[len(words)-1] += w
		default:
			words = append(words, w)
		}
	}
	bad := errors.New("when? e.g. 30m, 2h, 15:30, 3pm, tomorrow, tomorrow 8:30, fri 14")
	var t time.Time
	switch len(words) {
	case 1:
		w := words[0]
		if reDuration.MatchString(w) {
			d, _ := parseDuration(w)
			t = now.Add(d)
			break
		}
		if day, ok := laterDay(w, 9, 0, now); ok { // before parseUntil, whose "tomorrow" is midnight
			t = day
			break
		}
		if u, ok := parseUntil(w, now); ok {
			t = u
			break
		}
		return time.Time{}, bad
	case 2:
		h, mi, bare, ok := parseClock(words[1])
		if !ok {
			return time.Time{}, bad
		}
		if bare && h >= 1 && h < 7 {
			h += 12
		}
		if t, ok = laterDay(words[0], h, mi, now); !ok {
			return time.Time{}, bad
		}
	default:
		return time.Time{}, bad
	}
	if !t.After(now) {
		return time.Time{}, errors.New("that time has passed")
	}
	if t.Sub(now) > maxSchedule {
		return time.Time{}, errors.New("at most 120 days ahead (Slack's limit)")
	}
	return t, nil
}

// laterDay is h:mi on "today", "tomorrow" or the next given weekday (today
// too, when that time is still ahead).
func laterDay(word string, h, mi int, now time.Time) (time.Time, bool) {
	y, mo, d := now.Date()
	at := func(days int) time.Time { return time.Date(y, mo, d+days, h, mi, 0, 0, now.Location()) }
	switch word {
	case "today":
		return at(0), true
	case "tomorrow", "tmrw":
		return at(1), true
	}
	wd, ok := weekdays[word]
	if !ok {
		return time.Time{}, false
	}
	for i := 0; ; i++ {
		if t := at(i); t.Weekday() == wd && t.After(now) {
			return t, true
		}
	}
}

// fmtWhen says when a scheduled message goes out, as briefly as is clear.
func fmtWhen(t, now time.Time, layout string) string {
	y, mo, d := now.Date()
	midnight := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	switch days := int(t.Sub(midnight).Hours() / 24); {
	case t.Before(midnight):
		return t.Format("Mon 2 Jan " + layout)
	case days == 0:
		return "today " + t.Format(layout)
	case days == 1:
		return "tomorrow " + t.Format(layout)
	case days < 7:
		return t.Format("Mon " + layout)
	}
	return t.Format("Mon 2 Jan " + layout)
}

// cmdLater schedules what is in the composer.
func (m *Model) cmdLater(args string) tea.Cmd {
	conv, thread := m.inputTarget()
	switch {
	case conv == "":
		return m.setStatus("open a conversation first", false)
	case m.editing != nil:
		return m.setStatus("finish or cancel the edit first", false)
	case len(m.attach) > 0:
		return m.setStatus("images can't be scheduled — send them now, or ctrl+x to drop them", true)
	}
	text := strings.TrimSpace(m.input.Value())
	if text == "" && m.forwardFor(conv) == nil {
		return m.setStatus("write the message first, then :later 9:00 (or ctrl+s while writing)", false)
	}
	at, err := parseLater(args, clockNow())
	if err != nil {
		return m.setStatus(err.Error(), true)
	}
	encoded := m.outgoing(conv, text)
	m.input.Reset()
	m.fitInput()
	m.complete = nil
	m.insert = false
	m.input.Blur()
	return func() tea.Msg {
		s, err := m.client.Schedule(m.ctx, conv, encoded, thread, at)
		return scheduledMsg{s: s, threadTS: thread, text: text, err: err}
	}
}

// laterFromInsert is ctrl+s while writing: the command line, ready for a time.
func (m *Model) laterFromInsert() tea.Cmd {
	cmd := m.openCmd()
	m.cmd.input.SetValue("later ")
	m.cmd.input.CursorEnd()
	return cmd
}

func (m *Model) loadScheduled() tea.Cmd {
	return func() tea.Msg {
		list, err := m.client.ScheduledMessages(m.ctx)
		return scheduledListMsg{list: list, err: err}
	}
}

// upcoming is the scheduled messages not yet due; the ones whose time has
// come have been sent and show up as ordinary messages.
func (m *Model) upcoming() []backend.Scheduled {
	now := clockNow()
	var out []backend.Scheduled
	for _, s := range m.scheduled {
		if s.PostAt.After(now) {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) addScheduled(s backend.Scheduled) {
	i := 0
	for i < len(m.scheduled) && !m.scheduled[i].PostAt.After(s.PostAt) {
		i++
	}
	m.scheduled = append(m.scheduled[:i], append([]backend.Scheduled{s}, m.scheduled[i:]...)...)
}

func (m *Model) dropScheduled(id string) {
	for i, s := range m.scheduled {
		if s.ID == id {
			m.scheduled = append(m.scheduled[:i], m.scheduled[i+1:]...)
			return
		}
	}
}

func (m *Model) scheduledResult(msg scheduledMsg) tea.Cmd {
	if msg.err != nil {
		if m.input.Value() == "" {
			m.input.SetValue(msg.text)
			m.fitInput()
		}
		return m.setStatus("schedule failed: "+msg.err.Error(), true)
	}
	if msg.threadTS != "" {
		m.schedThread[msg.s.ID] = msg.threadTS
	}
	m.addScheduled(msg.s)
	return m.setResult("scheduled for " + fmtWhen(msg.s.PostAt, clockNow(), m.cfg.TimeFormat))
}

func (m *Model) openScheduled() tea.Cmd {
	m.closeSearch()
	m.pins, m.shared, m.chans, m.members = nil, nil, nil, nil
	m.insert = false
	m.input.Blur()
	m.sched = &schedState{loading: true}
	return m.loadScheduled()
}

func (m *Model) scheduledListResult(msg scheduledListMsg) tea.Cmd {
	if s := m.sched; s != nil {
		s.loading = false
		s.err = ""
		if msg.err != nil {
			s.err = msg.err.Error()
		}
	}
	if msg.err != nil {
		return nil // shown in the pane when it is open; background refreshes stay quiet
	}
	m.scheduled = msg.list
	if s := m.sched; s != nil {
		s.sel = min(s.sel, max(len(m.schedRows())-1, 0))
	}
	return nil
}

// openAt opens a conversation, and the thread when there is one.
func (m *Model) openAt(conv, threadTS string) tea.Cmd {
	cmd := m.open(conv)
	if threadTS == "" {
		return cmd
	}
	parent := backend.Message{TS: threadTS}
	if l := m.lists[conv]; l != nil {
		if i := msgIndex(l.msgs, threadTS); i >= 0 {
			parent = l.msgs[i]
		}
	}
	return tea.Batch(cmd, m.openThread(parent))
}

func (m *Model) schedKey(k tea.KeyMsg) tea.Cmd {
	s := m.sched
	list := m.schedRows()
	n := len(list)
	var cur *backend.Scheduled
	if n > 0 {
		s.sel = min(max(s.sel, 0), n-1)
		cur = &list[s.sel]
	}
	switch k.String() {
	case "esc":
		m.sched = nil
		return nil
	case "q":
		return m.quit()
	case ":":
		return m.openCmd()
	case "ctrl+k":
		m.sched = nil
		m.switcher = m.newSwitcher()
		return nil
	case "j", "down":
		s.sel++
	case "k", "up":
		s.sel--
	case "g", "home":
		s.sel = 0
	case "G", "end":
		s.sel = n - 1
	case "R":
		s.loading = true
		return m.loadScheduled()
	case "enter", "l", "right":
		if cur == nil || m.convs[cur.Channel] == nil {
			return nil
		}
		m.sched = nil
		return m.openAt(cur.Channel, m.schedThread[cur.ID])
	case "e":
		if cur == nil {
			return nil
		}
		if m.convs[cur.Channel] == nil {
			return m.setStatus("not in that conversation any more — d cancels it", false)
		}
		if strings.TrimSpace(m.input.Value()) != "" || len(m.attach) > 0 {
			return m.setStatus("the composer isn't empty — send or clear it first", false)
		}
		sm := *cur
		m.dropScheduled(sm.ID)
		thread := m.schedThread[sm.ID]
		return func() tea.Msg {
			return unscheduledMsg{s: sm, toInput: true, threadTS: thread, err: m.client.Unschedule(m.ctx, sm.Channel, sm.ID)}
		}
	case "d", "D", "x":
		if cur == nil {
			return nil
		}
		sm := *cur
		m.confirm = &confirmState{
			prompt: "Cancel the message for " + fmtWhen(sm.PostAt, clockNow(), m.cfg.TimeFormat) + "?", detail: "it won't be sent", yesLabel: "cancel it", noLabel: "keep",
			yes: func() tea.Cmd {
				m.dropScheduled(sm.ID)
				return func() tea.Msg {
					return unscheduledMsg{s: sm, err: m.client.Unschedule(m.ctx, sm.Channel, sm.ID)}
				}
			},
		}
		return nil
	}
	s.sel = min(max(s.sel, 0), max(n-1, 0))
	return nil
}

func (m *Model) unscheduledResult(msg unscheduledMsg) tea.Cmd {
	if msg.err != nil {
		m.addScheduled(msg.s)
		return m.setStatus(msg.err.Error(), true)
	}
	delete(m.schedThread, msg.s.ID)
	if !msg.toInput {
		return m.setStatus("scheduled message cancelled", false)
	}
	// Back into the composer of the conversation (or thread) it was for.
	m.sched = nil
	cmd := m.openAt(msg.s.Channel, msg.threadTS)
	text, tokens := render.Editable(msg.s.Text, m)
	for k, v := range tokens {
		m.composeTokens[k] = v
	}
	m.input.SetValue(text)
	m.input.CursorEnd()
	m.fitInput()
	m.insert = true
	return tea.Batch(cmd, m.input.Focus(), m.setStatus("unscheduled — enter sends now, ctrl+s schedules again", false))
}

// scheduledIn counts the messages waiting to go out in a conversation, for
// its header.
func (m *Model) scheduledIn(conv string) int {
	n := 0
	for _, s := range m.upcoming() {
		if s.Channel == conv {
			n++
		}
	}
	return n
}

// renderScheduled is the Scheduled pane: what is waiting, grouped by
// conversation.
func (m *Model) renderScheduled(w, h int) string {
	s := m.sched
	list := m.schedRows()
	lw := m.paneListW(w, true)
	p := paneSpec{title: frameTitle("Scheduled", true), top: &s.top}
	if len(list) > 0 {
		p.right = sty.Faint.Render(fmt.Sprintf("%d waiting", len(list)))
	}
	if s.err != "" {
		p.notes = []string{" " + sty.Err.Render(s.err)}
	}
	p.empty = "nothing scheduled — write a message, then :later"
	if s.loading {
		p.empty = "loading…"
	}
	if len(list) == 0 {
		return m.renderPane(p, w, h)
	}

	now := clockNow()
	sel := min(max(s.sel, 0), len(list)-1)
	for i, sm := range list {
		if i == 0 || list[i-1].Channel != sm.Channel {
			where := sm.Channel
			if c := m.convs[sm.Channel]; c != nil {
				where = m.convTitle(c)
			}
			if i > 0 {
				p.lines = append(p.lines, "")
			}
			p.lines = append(p.lines, group(where, ""))
		}
		if i == sel {
			p.selStart = len(p.lines)
		}
		line := sty.Warn.Render(ico.Scheduled + " " + fmtWhen(sm.PostAt, now, m.cfg.TimeFormat))
		if m.schedThread[sm.ID] != "" {
			line += "  " + sty.Thread.Render("in thread")
		}
		block := []string{line}
		text := strings.ReplaceAll(render.Mrkdwn(sm.Text, m, sty.Md), "\n", " ")
		wrapped := wrap(text, lw-4)
		if len(wrapped) > 3 {
			wrapped = append(wrapped[:3:3], sty.Faint.Render("…"))
		}
		block = append(block, prefixAll(wrapped, "  ")...)
		p.lines = append(p.lines, blockRows(block, lw, i == sel, true)...)
		if i == sel {
			p.selEnd = len(p.lines)
		}
	}
	sm := list[sel]
	p.preview = func(w, h int) string {
		out := []string{sty.Warn.Render(ico.Scheduled + " " + fmtWhen(sm.PostAt, now, m.cfg.TimeFormat)), ""}
		out = append(out, wrap(render.Mrkdwn(sm.Text, m, sty.Md), w)...)
		return strings.Join(out[:min(len(out), h)], "\n")
	}
	return m.renderPane(p, w, h)
}

// schedRows are the scheduled messages as the pane lists them: grouped by
// conversation, the soonest conversation first, each by time.
func (m *Model) schedRows() []backend.Scheduled {
	list := m.upcoming()
	first := map[string]int{}
	for i, s := range list {
		if _, ok := first[s.Channel]; !ok {
			first[s.Channel] = i
		}
	}
	slices.SortStableFunc(list, func(a, b backend.Scheduled) int { return first[a.Channel] - first[b.Channel] })
	return list
}
