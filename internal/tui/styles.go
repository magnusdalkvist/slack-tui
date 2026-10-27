package tui

import (
	"hash/fnv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// Styles is every style the views use, built once from the Theme. Views
// never pick colours themselves.
type Styles struct {
	theme Theme

	Muted, Faint, Normal lipgloss.Style
	Unread, Header       lipgloss.Style
	Title, Topic         lipgloss.Style
	Bar, Rule            lipgloss.Style
	Time, System         lipgloss.Style
	NewLine, Date        lipgloss.Style
	Thread               lipgloss.Style
	Reaction, ReactSelf  lipgloss.Style
	ModeNormal           lipgloss.Style
	ModeInsert           lipgloss.Style
	PillWarn, PillErr    lipgloss.Style // SEARCH, CONFIRM; ERROR and the offline chip
	PillCmd              lipgloss.Style // ":"
	OK, Warn, Err        lipgloss.Style
	Key                  lipgloss.Style
	AppTag               lipgloss.Style // "APP" after a bot's name
	Match                lipgloss.Style // what a filter matched
	KindTag              lipgloss.Style // a file's kind: [PDF]
	FrameTitle           lipgloss.Style // an idle frame's title
	FrameTitleFocus      lipgloss.Style
	sel, selFocus        lipgloss.Style // selection backgrounds
	Md                   render.Styles

	senders []lipgloss.Style
}

// sty is the active Styles; useTheme replaces it.
var sty = NewStyles(Dark())

func useTheme(t Theme) { sty = NewStyles(t) }

func NewStyles(t Theme) Styles {
	fg := func(c lipgloss.TerminalColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	// A solid pill: onAccent on c, or reverse video without colours.
	pill := func(c lipgloss.TerminalColor) lipgloss.Style {
		if t.NoColor {
			return lipgloss.NewStyle().Reverse(true).Bold(true).Padding(0, 1)
		}
		return lipgloss.NewStyle().Foreground(t.OnAccent).Background(c).Bold(true).Padding(0, 1)
	}
	on := func(f, b lipgloss.TerminalColor) lipgloss.Style {
		if t.NoColor {
			return lipgloss.NewStyle().Reverse(true)
		}
		return lipgloss.NewStyle().Foreground(f).Background(b)
	}
	s := Styles{
		theme:      t,
		Muted:      fg(t.Muted),
		Faint:      fg(t.Faint),
		Normal:     fg(t.Fg),
		Unread:     fg(t.Fg).Bold(true),
		Header:     fg(t.Faint).Bold(true),
		Title:      fg(t.Fg).Bold(true),
		Topic:      fg(t.Muted),
		Bar:        fg(t.Accent),
		Rule:       fg(t.Rule),
		Time:       fg(t.Faint),
		System:     fg(t.Faint).Italic(true),
		NewLine:    fg(t.Mention),
		Date:       fg(t.Faint).Bold(true),
		Thread:     fg(t.Accent),
		Reaction:   on(t.Fg, t.Raised).Padding(0, 1),
		ReactSelf:  on(t.Accent, t.SelFocus).Bold(true).Padding(0, 1),
		ModeNormal: pill(t.Accent),
		ModeInsert: pill(t.OK),
		PillWarn:   pill(t.Warn),
		PillErr:    pill(t.Mention),
		PillCmd:    pill(t.Fg),
		OK:         fg(t.OK),
		Warn:       fg(t.Warn),
		Err:        fg(t.Mention),
		Key:        fg(t.Fg).Bold(true),

		AppTag:          on(t.Muted, t.Sel).Padding(0, 1),
		Match:           fg(t.Accent).Bold(true),
		KindTag:         on(t.Fg, t.Sel).Bold(true).Padding(0, 1),
		FrameTitle:      fg(t.Muted).Bold(true),
		FrameTitleFocus: fg(t.Accent).Bold(true).Reverse(t.NoColor), // no border colour to show focus
		sel:             on(t.Fg, t.Sel),
		selFocus:        on(t.Fg, t.SelFocus),
	}
	s.Md = render.Styles{
		Bold:         lipgloss.NewStyle().Bold(true),
		Italic:       lipgloss.NewStyle().Italic(true),
		Strike:       lipgloss.NewStyle().Strikethrough(true),
		Code:         on(t.Self, t.Raised),
		CodeBlock:    on(t.Fg, t.Raised),
		Quote:        fg(t.Muted),
		Link:         fg(t.Accent).Underline(true),
		Mention:      fg(t.Accent).Bold(true),
		SelfMention:  fg(t.Self).Bold(true),
		Channel:      fg(t.Accent),
		UnknownEmoji: fg(t.Faint),
		CustomEmoji:  on(t.Faint, t.Raised),
	}
	for _, c := range t.Senders {
		s.senders = append(s.senders, fg(c).Bold(true))
	}
	return s
}

// Sender is the bold, hashed colour of a sender's name.
func (s Styles) Sender(key string) lipgloss.Style {
	h := fnv.New32a()
	h.Write([]byte(key))
	return s.senders[h.Sum32()%uint32(len(s.senders))]
}

// SelBgOn and SelFocusBgOn open the selection background of an unfocused
// and a focused frame.
func (s Styles) SelBgOn() string      { return bgPrefix(s.sel) }
func (s Styles) SelFocusBgOn() string { return bgPrefix(s.selFocus) }

// bgPrefix is the escape that opens st's background only (or reverse
// video without colours), so the row's own colours show through.
func bgPrefix(st lipgloss.Style) string {
	if st.GetReverse() {
		return sgrPrefix(lipgloss.NewStyle().Reverse(true))
	}
	return sgrPrefix(lipgloss.NewStyle().Background(st.GetBackground()))
}

// SelfBar and PinBar are the bar column of messages that mention us and
// of pinned ones; the tints go behind their rows.
func (s Styles) SelfBar() string       { return fgStyle(s.theme.Self).Render("▌") }
func (s Styles) PinBar() string        { return fgStyle(s.theme.Warn).Render("▌") }
func (s Styles) MentionTintOn() string { return bgOn(s.theme.MentionTint) }
func (s Styles) PinTintOn() string     { return bgOn(s.theme.PinTint) }
func (s Styles) RaisedBgOn() string    { return bgOn(s.theme.Raised) }

func bgOn(c lipgloss.TerminalColor) string {
	if _, none := c.(lipgloss.NoColor); none {
		return ""
	}
	return sgrPrefix(lipgloss.NewStyle().Background(c))
}

// SelOff closes a selection background (or reverse video).
const SelOff = "\x1b[27;49m"

// HitOn and HitOff mark search matches inside already styled text.
func (s Styles) HitOn() string {
	return sgrPrefix(fgStyle(s.theme.Self).Background(s.theme.Raised).Bold(true))
}

const HitOff = "\x1b[22;39;49m"

func fgStyle(c lipgloss.TerminalColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// sgrPrefix is the escape sequence a style opens with.
func sgrPrefix(st lipgloss.Style) string {
	out := st.Render("x")
	if i := strings.Index(out, "x"); i > 0 {
		return out[:i]
	}
	return ""
}

// senderKey is who a message is from, for its name colour and grouping: the
// person when there is one, since messages people send through an app (this
// one included) also carry the app's bot_id; otherwise the bot.
func senderKey(msg backend.Message) string {
	if msg.User != "" {
		return msg.User
	}
	return msg.BotName + msg.BotID
}
