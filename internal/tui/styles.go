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
	Active, Badge        lipgloss.Style
	Title, Topic         lipgloss.Style
	Bar, BarDim, Rule    lipgloss.Style
	Time, System         lipgloss.Style
	NewLine, Date        lipgloss.Style
	Thread               lipgloss.Style
	Reaction, ReactSelf  lipgloss.Style
	ModeNormal           lipgloss.Style
	ModeInsert           lipgloss.Style
	OK, Warn, Err        lipgloss.Style
	Box, Key             lipgloss.Style
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
		Active:     on(t.Fg, t.Sel),
		Badge:      pill(t.Mention),
		Title:      fg(t.Fg).Bold(true),
		Topic:      fg(t.Muted),
		Bar:        fg(t.Accent),
		BarDim:     fg(t.Rule),
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
		OK:         fg(t.OK),
		Warn:       fg(t.Warn),
		Err:        fg(t.Mention),
		Box:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent).Padding(0, 1),
		Key:        fg(t.Fg).Bold(true),
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

// SelOn and SelOff wrap content that sets its own foreground (custom emoji
// images) in the selection background, as raw SGR.
func (s Styles) SelOn() string { return sgrPrefix(s.Active) }

const SelOff = "\x1b[27;49m"

// HitOn and HitOff mark search matches inside already styled text.
func (s Styles) HitOn() string { return sgrPrefix(fgStyle(s.theme.Self).Bold(true)) }

const HitOff = "\x1b[22;39m"

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
