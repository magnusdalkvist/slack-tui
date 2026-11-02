package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Frames and selection (DESIGN.md §4, §5): every pane is a rounded frame
// with its labels set into the top border, and every list marks its
// selected row the same way.

// RenderFrame draws content inside a w×h rounded frame (outer size). left
// is the title, right the meta; both go into the top border. The border is
// accent when focused and rule otherwise.
func RenderFrame(content string, w, h int, left, right string, focused bool) string {
	border := sty.Rule
	if focused {
		border = sty.Bar
	}
	return frameWith(content, w, h, left, right, border)
}

// frameWith is RenderFrame with any border style (the composer's follows
// the mode).
func frameWith(content string, w, h int, left, right string, border lipgloss.Style) string {
	if w < 4 || h < 2 {
		return fit(content, max(w, 0), max(h, 0))
	}
	inner := w - 2
	out := make([]string, 0, h)
	out = append(out, topBorder(w, left, right, border))
	side := border.Render("│")
	for _, l := range strings.Split(fit(content, inner, h-2), "\n") {
		out = append(out, side+l+side)
	}
	if h > 2 {
		out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	}
	return strings.Join(out, "\n")
}

// topBorder is "╭ left ──── right ╮": each label padded by a space, the
// left one first to give way, the right one dropped when there is no room.
func topBorder(w int, left, right string, border lipgloss.Style) string {
	room := w - 2 // between the corners
	pad := func(s string) string {
		if s == "" {
			return ""
		}
		return " " + s + " "
	}
	l, r := pad(left), pad(right)
	if ansi.StringWidth(l)+ansi.StringWidth(r)+1 > room {
		r = ""
	}
	if ansi.StringWidth(l)+1 > room {
		l = pad(ansi.Truncate(left, max(0, room-3), "…"))
	}
	if ansi.StringWidth(l)+ansi.StringWidth(r) > room {
		r = ""
	}
	if ansi.StringWidth(l) > room {
		l = ""
	}
	fill := room - ansi.StringWidth(l) - ansi.StringWidth(r)
	return border.Render("╭") + l + border.Render(strings.Repeat("─", fill)) + r + border.Render("╮")
}

// frameTitle is a frame's left label: the title bold, accent when the
// frame has focus and muted otherwise, then its context as given.
func frameTitle(title string, focused bool, context ...string) string {
	st := sty.FrameTitle
	if focused {
		st = sty.FrameTitleFocus
	}
	return st.Render(title) + frameContext(context)
}

// frameContext is what follows a frame's title: each part two cells on.
func frameContext(context []string) string {
	out := ""
	for _, c := range context {
		if c != "" {
			out += "  " + c
		}
	}
	return out
}

// selLine marks one row of a list: the bar column (1 cell) then the row,
// padded to w. A selected row gets the selection background across the
// whole width, and an accent ▌ when its frame has focus.
func selLine(row string, w int, selected, focused bool) string {
	return flagLine(" ", row, w, selected, focused)
}

// flagLine is selLine for rows that carry an edge flag in the bar column
// (the sidebar's unread markers). The focused selection's ▌ covers it.
func flagLine(flag, row string, w int, selected, focused bool) string {
	if !selected {
		return flag + padRight(row, w-1)
	}
	bar, bg := flag, sty.SelBgOn()
	if focused {
		bar, bg = sty.Bar.Render("▌"), sty.SelFocusBgOn()
	}
	return bar + paintBg(row, w-1, bg)
}

// selLines is selLine over every line of a block.
func selLines(rows []string, w int, selected, focused bool) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = selLine(r, w, selected, focused)
	}
	return out
}

func padRight(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// clearsBg is true for SGR parameters that reset the background (0, 49)
// or reverse video (27), skipping the arguments of extended colours.
func clearsBg(params string) bool {
	if params == "" {
		return true
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		switch ps[i] {
		case "", "0", "49", "27":
			return true
		case "38", "48", "58":
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
		}
	}
	return false
}

// paintBg puts the background bg (an SGR prefix) behind s, padded to w,
// putting it back after every reset inside s so styled spans keep it.
func paintBg(s string, w int, bg string) string {
	s = padRight(s, w)
	if bg == "" {
		return s
	}
	s = sgr.ReplaceAllStringFunc(s, func(r string) string {
		if clearsBg(r[2 : len(r)-1]) {
			return r + bg
		}
		return r
	})
	return bg + s + SelOff
}
