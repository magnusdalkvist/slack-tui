package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// picker is the emoji chooser for reactions.
type picker struct {
	input   textinput.Model
	conv    string
	msg     backend.Message
	names   []string // every candidate
	custom  map[string]bool
	matches []string // standard emoji first, then custom ones
	split   int      // matches[split:] are custom (the CUSTOM group)
	sel     int
}

// commonReactions open the picker before anything is typed.
var commonReactions = []string{"+1", "heart", "joy", "eyes", "white_check_mark", "tada", "pray", "fire", "raised_hands", "100", "thinking_face", "rocket"}

const pickerRows = 10

type reactedMsg struct {
	ev  backend.EvReaction
	err error
}

func (m *Model) newPicker(conv string, msg backend.Message) *picker {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "react with…"
	ti.Focus()
	p := &picker{input: ti, conv: conv, msg: msg, custom: map[string]bool{}}
	p.names = render.EmojiNames()
	for n := range m.emoji {
		p.names = append(p.names, n)
		p.custom[n] = true
	}
	slices.Sort(p.names)
	p.names = slices.Compact(p.names)
	p.filter()
	return p
}

func (p *picker) filter() {
	q := strings.ToLower(strings.Trim(strings.TrimSpace(p.input.Value()), ":"))
	p.sel = 0
	var std, custom []string
	if q == "" {
		// FREQUENT: reactions already on the message, then the usual ones.
		for _, r := range p.msg.Reactions {
			std = append(std, r.Name)
		}
		for _, n := range commonReactions {
			if !slices.Contains(std, n) {
				std = append(std, n)
			}
		}
		// CUSTOM: the workspace's own, by name.
		for _, n := range p.names {
			if p.custom[n] && !slices.Contains(std, n) && len(custom) < 200 {
				custom = append(custom, n)
			}
		}
	} else {
		for _, i := range fuzzyRank(q, p.names) {
			if n := p.names[i]; p.custom[n] {
				custom = append(custom, n)
			} else {
				std = append(std, n)
			}
			if len(std)+len(custom) == 200 {
				break
			}
		}
	}
	p.matches = append(std, custom...)
	p.split = len(std)
}

// visible is the slice of matches on screen, so their images get loaded.
func (p *picker) visible() []string {
	start := max(0, p.sel-pickerRows+1)
	return p.matches[start:min(len(p.matches), start+pickerRows+1)]
}

func (m *Model) openPicker(conv string, msg backend.Message) tea.Cmd {
	m.picker = m.newPicker(conv, msg)
	return nil
}

func (m *Model) pickerKey(k tea.KeyMsg) tea.Cmd {
	p := m.picker
	switch k.String() {
	case "esc":
		m.picker = nil
		return nil
	case "enter":
		m.picker = nil
		if len(p.matches) == 0 {
			return nil
		}
		return m.toggleReaction(p.conv, p.msg.TS, p.matches[p.sel])
	case "down", "ctrl+n", "ctrl+j", "tab":
		p.sel = min(p.sel+1, max(len(p.matches)-1, 0))
		return nil
	case "up", "ctrl+p", "shift+tab":
		p.sel = max(p.sel-1, 0)
		return nil
	}
	var cmd tea.Cmd
	before := p.input.Value()
	p.input, cmd = p.input.Update(k)
	if p.input.Value() != before {
		p.filter()
	}
	return cmd
}

// hasReacted reports whether we already reacted to ts with name.
func (m *Model) hasReacted(conv, ts, name string) bool {
	for _, l := range []*msgList{m.lists[conv], m.threadList(conv)} {
		if l == nil {
			continue
		}
		for _, msg := range l.msgs {
			if msg.TS != ts {
				continue
			}
			for _, r := range msg.Reactions {
				if r.Name == name && slices.Contains(r.Users, m.client.SelfID) {
					return true
				}
			}
			return false
		}
	}
	return false
}

func (m *Model) threadList(conv string) *msgList {
	if m.thread != nil && m.thread.conv == conv {
		return &m.thread.list
	}
	return nil
}

// toggleReaction adds the reaction, or removes it if we already have it.
// The change shows at once; the live event that follows is a no-op, and a
// failed call puts it back.
func (m *Model) toggleReaction(conv, ts, name string) tea.Cmd {
	ev := backend.EvReaction{Channel: conv, TS: ts, User: m.client.SelfID, Name: name, Added: !m.hasReacted(conv, ts, name)}
	m.applyReaction(ev)
	return func() tea.Msg {
		return reactedMsg{ev: ev, err: m.client.React(m.ctx, conv, ts, name, ev.Added)}
	}
}

// renderPicker is the React modal: FREQUENT (or the matches), then the
// workspace's CUSTOM emoji, each with its glyph and whether we added it.
func (m *Model) renderPicker() string {
	p := m.picker
	w := m.modalWidth(50)
	inner := w - 2
	p.input.Width = inner - 4
	q := strings.Trim(strings.TrimSpace(p.input.Value()), ":")
	rows := []string{" " + p.input.View(), modalRule(inner)}
	start := max(0, p.sel-pickerRows+1)
	for i := start; i < len(p.matches) && i < start+pickerRows; i++ {
		switch {
		case i == 0 && p.split > 0 && q == "":
			rows = append(rows, modalHeader("Frequent"))
		case i == p.split && i < len(p.matches):
			rows = append(rows, modalHeader("Custom"))
		}
		name := p.matches[i]
		row := " " + m.emojiChip(name) + " " + markMatch(":"+name+":", q, sty.Normal)
		meta := ""
		if m.hasReacted(p.conv, p.msg.TS, name) {
			meta = sty.OK.Render("✓ added")
		}
		rows = append(rows, modalRow(rightAlign(row, meta, inner), inner, i == p.sel))
	}
	if len(p.matches) == 0 {
		rows = append(rows, " "+sty.Muted.Render("no emoji match"))
	}
	right := "to " + m.senderName(p.msg) + " " + p.msg.Time().Format(m.cfg.TimeFormat)
	return modalBox(w, "React", right, rows)
}
