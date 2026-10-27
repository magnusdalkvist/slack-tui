package tui

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is the palette every view draws with (DESIGN.md §2). This is the
// only file that names colours; views use the Styles built from it.
type Theme struct {
	Name string

	Raised   lipgloss.TerminalColor // status bar, modals, chips, inline code
	Sel      lipgloss.TerminalColor // selection in an unfocused frame
	SelFocus lipgloss.TerminalColor // selection in the focused frame, own reactions
	Fg       lipgloss.TerminalColor // body text, unread rows, hint keys
	Muted    lipgloss.TerminalColor // read rows, secondary text
	Faint    lipgloss.TerminalColor // times, meta, hint labels, placeholders
	Rule     lipgloss.TerminalColor // idle frame borders, separators
	Accent   lipgloss.TerminalColor // focus, links, selection bar
	OnAccent lipgloss.TerminalColor // text on solid pills and chips
	Mention  lipgloss.TerminalColor // needs you: flags, counts, errors
	Self     lipgloss.TerminalColor // about you, or matched
	OK       lipgloss.TerminalColor // live, active, typing
	Warn     lipgloss.TerminalColor // reconnecting, searching, pinned

	MentionTint lipgloss.TerminalColor // behind messages that mention you
	PinTint     lipgloss.TerminalColor // behind pinned messages

	// Senders colour names, by a hash of the user ID.
	Senders []lipgloss.TerminalColor

	// NoColor is NO_COLOR: no colours at all, only bold and reverse.
	NoColor bool

	// hex is each token's truecolor value, for the contrast check; bg is
	// the terminal background the theme assumes (never painted).
	hex map[string]string
}

// tok is one token: an xterm-256 index, its truecolor hex and a 16-colour
// index for terminals with neither.
type tok struct{ x256, hex, ansi string }

func (t tok) color() lipgloss.TerminalColor {
	return lipgloss.CompleteColor{TrueColor: t.hex, ANSI256: t.x256, ANSI: t.ansi}
}

func build(name string, toks map[string]tok, senders []tok) Theme {
	c := func(k string) lipgloss.TerminalColor { return toks[k].color() }
	t := Theme{
		Name: name, Raised: c("raised"), Sel: c("sel"), SelFocus: c("selFocus"), Fg: c("fg"),
		Muted: c("muted"), Faint: c("faint"), Rule: c("rule"), Accent: c("accent"),
		OnAccent: c("onAccent"), Mention: c("mention"), Self: c("self"), OK: c("ok"), Warn: c("warn"),
		MentionTint: c("mentionTint"), PinTint: c("pinTint"),
		hex: map[string]string{},
	}
	for k, v := range toks {
		t.hex[k] = v.hex
	}
	for _, s := range senders {
		t.Senders = append(t.Senders, s.color())
	}
	return t
}

func Dark() Theme {
	return build("dark", map[string]tok{
		"bg":          {"233", "#121315", "0"},
		"raised":      {"234", "#1c1d20", "0"},
		"sel":         {"235", "#26282c", "8"},
		"selFocus":    {"235", "#1b2a3e", "4"},
		"fg":          {"254", "#e4e4e4", "15"},
		"muted":       {"249", "#b2b2b2", "7"},
		"faint":       {"245", "#8a8a8a", "8"},
		"rule":        {"237", "#3a3a3a", "8"},
		"accent":      {"75", "#5fafff", "12"},
		"onAccent":    {"233", "#121315", "0"},
		"mention":     {"203", "#ff5f5f", "9"},
		"self":        {"221", "#ffd75f", "11"},
		"ok":          {"114", "#87d787", "10"},
		"warn":        {"215", "#ffaf5f", "3"},
		"mentionTint": {"235", "#2a2618", "0"},
		"pinTint":     {"234", "#1d1b14", "0"},
	}, []tok{
		{"177", "#d787ff", "13"}, {"79", "#5fd7af", "14"}, {"211", "#ff87af", "13"}, {"147", "#afafff", "12"},
		{"180", "#d7af87", "3"}, {"141", "#af87ff", "5"}, {"116", "#87d7d7", "6"}, {"174", "#d78787", "1"},
		{"187", "#d7d7af", "7"}, {"218", "#ffafd7", "13"},
	})
}

func Light() Theme {
	return build("light", map[string]tok{
		"bg":          {"231", "#ffffff", "15"},
		"raised":      {"255", "#eeeeee", "7"},
		"sel":         {"254", "#e4e4e4", "7"},
		"selFocus":    {"189", "#d7e7fb", "14"},
		"fg":          {"234", "#1c1c1c", "0"},
		"muted":       {"238", "#444444", "8"},
		"faint":       {"242", "#6c6c6c", "8"},
		"rule":        {"251", "#c6c6c6", "7"},
		"accent":      {"26", "#005fd7", "4"},
		"onAccent":    {"231", "#ffffff", "15"},
		"mention":     {"160", "#d70000", "1"},
		"self":        {"94", "#875f00", "3"},
		"ok":          {"22", "#007a00", "2"},
		"warn":        {"130", "#a05500", "3"},
		"mentionTint": {"230", "#fbf3dc", "7"},
		"pinTint":     {"255", "#fbf0e4", "7"},
	}, []tok{
		{"91", "#8700af", "5"}, {"29", "#00875f", "2"}, {"125", "#af005f", "5"}, {"54", "#5f0087", "5"},
		{"97", "#875faf", "5"}, {"30", "#008787", "6"}, {"58", "#5f5f00", "3"}, {"60", "#5f5f87", "4"},
		{"23", "#005f5f", "6"}, {"89", "#87005f", "1"},
	})
}

// ANSI follows the terminal's own 16-colour palette, so it matches
// whatever scheme the terminal is set to, light or dark.
func ANSI() Theme {
	c := func(s string) lipgloss.TerminalColor { return lipgloss.Color(s) }
	t := Theme{
		Name: "ansi", Raised: c("0"), Sel: c("8"), SelFocus: c("4"), Fg: lipgloss.NoColor{},
		Muted: c("7"), Faint: c("8"), Rule: c("8"), Accent: c("12"), OnAccent: c("0"),
		Mention: c("9"), Self: c("11"), OK: c("10"), Warn: c("3"),
		MentionTint: lipgloss.NoColor{}, PinTint: lipgloss.NoColor{},
		hex: map[string]string{},
	}
	for _, s := range []string{"13", "14", "5", "6", "12", "3", "1", "2", "4", "7"} {
		t.Senders = append(t.Senders, c(s))
	}
	return t
}

// Auto picks Dark or Light by the terminal's background.
func Auto() Theme {
	if lipgloss.HasDarkBackground() {
		return Dark()
	}
	return Light()
}

// noColor is NO_COLOR: every token unset.
func noColor() Theme {
	n := lipgloss.NoColor{}
	t := Theme{
		Name: "none", Raised: n, Sel: n, SelFocus: n, Fg: n, Muted: n, Faint: n, Rule: n,
		Accent: n, OnAccent: n, Mention: n, Self: n, OK: n, Warn: n, MentionTint: n, PinTint: n,
		Senders: []lipgloss.TerminalColor{n}, NoColor: true, hex: map[string]string{},
	}
	return t
}

// LoadTheme is the theme for the config's [theme] table: name is auto,
// dark, light or ansi; overrides set single tokens to a hex value or a
// 256-colour index. NO_COLOR wins over both. The warnings are overrides
// that were not understood or that are hard to read.
func LoadTheme(name string, overrides map[string]string) (Theme, []string) {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return noColor(), nil
	}
	var t Theme
	var warns []string
	switch strings.ToLower(name) {
	case "", "auto":
		t = Auto()
	case "dark":
		t = Dark()
	case "light":
		t = Light()
	case "ansi":
		t = ANSI()
	default:
		t = Auto()
		warns = append(warns, fmt.Sprintf("theme %q: not auto, dark, light or ansi", name))
	}
	for k, v := range overrides {
		p := t.token(k)
		if p == nil {
			warns = append(warns, fmt.Sprintf("theme override %q: no such token", k))
			continue
		}
		hex, ok := toHex(v)
		if !ok {
			warns = append(warns, fmt.Sprintf("theme override %s = %q: not a hex colour or 0-255", k, v))
			continue
		}
		*p = lipgloss.Color(v)
		t.hex[k] = hex
	}
	for k := range overrides {
		warns = append(warns, t.checkContrast(k)...)
	}
	return t, warns
}

func (t *Theme) token(name string) *lipgloss.TerminalColor {
	switch name {
	case "raised":
		return &t.Raised
	case "sel":
		return &t.Sel
	case "selFocus":
		return &t.SelFocus
	case "fg":
		return &t.Fg
	case "muted":
		return &t.Muted
	case "faint":
		return &t.Faint
	case "rule":
		return &t.Rule
	case "accent":
		return &t.Accent
	case "onAccent":
		return &t.OnAccent
	case "mention":
		return &t.Mention
	case "self":
		return &t.Self
	case "ok":
		return &t.OK
	case "warn":
		return &t.Warn
	case "mentionTint":
		return &t.MentionTint
	case "pinTint":
		return &t.PinTint
	}
	return nil
}

// textTokens must reach 4.5:1 on bg and raised.
var textTokens = []string{"fg", "muted", "faint", "accent", "mention", "self", "ok", "warn"}

// checkContrast warns when the token k (just overridden) makes text hard to
// read: a text token against bg and raised, or raised against every text token.
func (t Theme) checkContrast(k string) []string {
	var warns []string
	check := func(fg, bg string) {
		a, b := t.hex[fg], t.hex[bg]
		if a == "" || b == "" {
			return
		}
		if r := contrast(a, b); r < 4.5 {
			warns = append(warns, fmt.Sprintf("theme: %s on %s is %.1f:1, below 4.5:1", fg, bg, r))
		}
	}
	for _, fg := range textTokens {
		if fg == k {
			check(fg, "bg")
			check(fg, "raised")
		} else if k == "raised" {
			check(fg, "raised")
		}
	}
	return warns
}

// toHex reads "#rrggbb" or a 256-colour index.
func toHex(v string) (string, bool) {
	if strings.HasPrefix(v, "#") && len(v) == 7 {
		if _, err := strconv.ParseUint(v[1:], 16, 32); err == nil {
			return strings.ToLower(v), true
		}
		return "", false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 255 {
		return "", false
	}
	return xterm256Hex(n), true
}

// xterm256Hex is the standard xterm value of a 256-colour index.
func xterm256Hex(n int) string {
	base := []string{"#000000", "#800000", "#008000", "#808000", "#000080", "#800080", "#008080", "#c0c0c0",
		"#808080", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff"}
	switch {
	case n < 16:
		return base[n]
	case n < 232:
		n -= 16
		lvl := func(i int) int {
			if i == 0 {
				return 0
			}
			return 55 + 40*i
		}
		return fmt.Sprintf("#%02x%02x%02x", lvl(n/36), lvl(n/6%6), lvl(n%6))
	default:
		g := 8 + 10*(n-232)
		return fmt.Sprintf("#%02x%02x%02x", g, g, g)
	}
}

// contrast is the WCAG contrast ratio of two hex colours.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	ch := func(c uint64) float64 {
		s := float64(c) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(v>>16&0xff) + 0.7152*ch(v>>8&0xff) + 0.0722*ch(v&0xff)
}
