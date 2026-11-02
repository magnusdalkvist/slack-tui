package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// `>` quotes a message into the composer; `f` forwards it: pick where it
// goes, add a note if you like, and it's sent with the message's link,
// which Slack shows as the original message.

// forwardState is a message waiting in a composer to be forwarded.
type forwardState struct {
	conv string // where it goes; "" while a new DM is being opened
	link string // the message's permalink, sent after the note
	from string // "Anna Jensen in #dev"
	text string // the message's first line, for the chip
}

// quoteReply puts the message, each line behind "> ", into the composer
// of the pane it's in, with the cursor below it for the reply.
func (m *Model) quoteReply(conv string, msg backend.Message, inThread bool) tea.Cmd {
	text, tokens := render.Editable(render.PlainLinks(ownText(msg)), m)
	text = strings.TrimSpace(text)
	if text == "" {
		return m.setStatus("nothing to quote: the message has no text", false)
	}
	if m.editing != nil {
		m.cancelEdit()
	}
	for k, v := range tokens {
		m.composeTokens[k] = v
	}
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString("> " + line + "\n")
	}
	if inThread {
		m.focus = fThread
	} else {
		m.focus = fMessages
	}
	m.input.SetValue(b.String() + m.input.Value())
	m.input.CursorEnd()
	m.fitInput()
	m.insert = true
	return m.input.Focus()
}

// startForward opens Jump to as "Forward to" for msg, in conversation conv.
func (m *Model) startForward(conv string, msg backend.Message) tea.Cmd {
	if systemSubtypes[msg.SubType] {
		return m.setStatus("that message can't be forwarded", false)
	}
	f := &forwardState{link: m.client.Permalink(conv, msg.TS, msg.ThreadTS), from: m.senderName(msg)}
	if c := m.convs[conv]; c != nil {
		where := m.convLabel(c)
		if !c.IsDM() {
			where = "#" + where
		}
		f.from += " in " + where
	}
	text, _ := render.Editable(render.PlainLinks(ownText(msg)), m)
	f.text, _, _ = strings.Cut(strings.TrimSpace(text), "\n")
	if f.text == "" && len(msg.Files) > 0 {
		f.text = msg.Files[0].Name
	}
	s := m.newSwitcher()
	s.forward = f
	s.input.Placeholder = "forward to a channel or person"
	// Unreads is first; it is no place to send anything.
	s.ids, s.labels, s.convCount = s.ids[1:], s.labels[1:], s.convCount-1
	s.filter()
	m.switcher = s
	return nil
}

// forwardTo opens the conversation picked in "Forward to" and its
// composer, the message waiting above the text.
func (m *Model) forwardTo(f *forwardState, id string) tea.Cmd {
	m.forward = f
	if uid, ok := strings.CutPrefix(id, personPrefix); ok {
		f.conv = "" // set once the DM is open (forwardOpened)
		return m.openDMWith([]string{uid})
	}
	f.conv = id
	return tea.Batch(m.open(id), m.startInsert(), m.setStatus("add a note, or enter to forward as is", false))
}

// forwardOpened is a new DM opened for a waiting forward.
func (m *Model) forwardOpened(id string) tea.Cmd {
	if m.forward == nil || m.forward.conv != "" {
		return nil
	}
	m.forward.conv = id
	return m.startInsert()
}

// forwardFor is the forward waiting in conv's composer, if any.
func (m *Model) forwardFor(conv string) *forwardState {
	if m.forward != nil && m.forward.conv == conv && conv != "" {
		return m.forward
	}
	return nil
}

// outgoing is the composer's text as Slack text, the forwarded message's
// link after it. The forward is used up.
func (m *Model) outgoing(conv, text string) string {
	out := render.Encode(text, m.composeTokens)
	if f := m.forwardFor(conv); f != nil {
		m.forward = nil
		if out != "" {
			out += "\n"
		}
		out += f.link
	}
	return out
}

// forwardChip is the line above the composer's text while a forward
// waits: "↪ Anna Jensen in #dev: the first line…".
func (m *Model) forwardChip(f *forwardState, width int) string {
	head := sty.Faint.Render("↪ ") + sty.Muted.Render(f.from)
	if f.text != "" {
		head += sty.Faint.Render(": " + f.text)
	}
	return ansi.Truncate(head, width, "…")
}

// forwardHere is the forward waiting in the composer being typed into.
func (m *Model) forwardHere() *forwardState {
	conv, _ := m.inputTarget()
	return m.forwardFor(conv)
}
