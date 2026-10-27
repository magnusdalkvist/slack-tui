package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Modals (DESIGN.md §9): short-lived, pick something and go. Centred, the
// top at about 18% of the height, on raised inside an accent frame with
// the title in its border; everything behind is redrawn in faint. An
// input row, a rule, then rows; no footer, the keys are in the status bar.

const modalMaxW = 76

// modalBox draws a modal w cells wide (outer) around its rows.
func modalBox(w int, title, right string, rows []string) string {
	inner := w - 2
	body := make([]string, len(rows))
	for i, r := range rows {
		body[i] = paintBg(r, inner, sty.RaisedBgOn())
	}
	if right != "" {
		right = sty.Faint.Render(right)
	}
	return frameWith(strings.Join(body, "\n"), w, len(rows)+2, sty.FrameTitleFocus.Render(title), right, sty.Bar)
}

// modalRule is the line under a modal's input.
func modalRule(inner int) string { return sty.Rule.Render(strings.Repeat("─", inner)) }

// modalRow is one row of a modal's list: the selection when sel, on
// raised otherwise.
func modalRow(row string, inner int, sel bool) string {
	if sel {
		return selLine(row, inner, true, true)
	}
	return " " + padRight(row, inner-1)
}

// modalHeader is a group's faint bold caps header.
func modalHeader(name string) string { return " " + sty.Header.Render(strings.ToUpper(name)) }

// rightAlign puts meta at the right end of a row inner cells wide,
// cutting the row short first.
func rightAlign(row, meta string, inner int) string {
	mw := ansi.StringWidth(meta)
	if mw == 0 {
		return row
	}
	room := inner - 2 - mw
	row = ansi.Truncate(row, max(room-1, 0), "…")
	return row + strings.Repeat(" ", max(1, room-ansi.StringWidth(row))) + meta
}

// overlayModal lays box over the screen body, which is dimmed to the
// rule colour so the modal stands out.
func (m *Model) overlayModal(body, box string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = dimText(l)
	}
	rows := strings.Split(box, "\n")
	bw := ansi.StringWidth(rows[0])
	x := max(0, (m.width-bw)/2)
	y := min(max(0, len(lines)*18/100), max(0, len(lines)-len(rows)))
	for i, l := range lines {
		if k := i - y; k >= 0 && k < len(rows) {
			lines[i] = sty.Dim.Render(cells(l, 0, x)) + rows[k] + sty.Dim.Render(cells(l, x+bw, m.width))
		} else {
			lines[i] = sty.Dim.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// cells is the plain text s from cell from up to cell to; a wide
// character cut in half becomes spaces.
func cells(s string, from, to int) string {
	var b strings.Builder
	at := 0
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		// A character and the zero-width marks after it (a variation
		// selector makes ❤ two cells wide) are measured together.
		j := i + 1
		for j < len(rs) && (rs[j] == '\uFE0F' || rs[j] == '\u200D' || unicode.Is(unicode.Mn, rs[j])) {
			j++
		}
		g := string(rs[i:j])
		i = j - 1
		w := ansi.StringWidth(g)
		switch {
		case at >= to:
		case at >= from && at+w <= to:
			b.WriteString(g)
		case at+w > from:
			b.WriteString(strings.Repeat(" ", min(at+w, to)-max(at, from)))
		}
		at += w
	}
	return b.String()
}

// dimText is a line without its styling, and with image placeholders
// (which only draw in their own colours) blanked to spaces.
func dimText(l string) string {
	plain := ansi.Strip(l)
	if !strings.ContainsRune(plain, kittyPlaceholder) {
		return plain
	}
	var b strings.Builder
	skip := false
	for _, r := range plain {
		switch {
		case r == kittyPlaceholder:
			b.WriteByte(' ')
			skip = true
		case skip && unicode.Is(unicode.Mn, r):
		default:
			skip = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// kittyPlaceholder is the kitty graphics protocol's placeholder rune.
const kittyPlaceholder = '\U0010EEEE'

// modalWidth is a modal's outer width: wanted, but no wider than the
// window allows or the spec's maximum.
func (m *Model) modalWidth(want int) int {
	return max(20, min(want, modalMaxW, m.width-4))
}
