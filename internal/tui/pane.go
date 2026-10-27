package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

// Panes (DESIGN.md §9): browse a list and act on it. A pane takes over
// the chat frame; the thread's place shows a Preview of the selected item
// (when there is room), and the pane's input sits in the composer frame.

// paneSpec is what a pane draws; renderPane frames and scrolls it.
type paneSpec struct {
	title, right string   // the frame's labels, styled
	lines        []string // the list at listW, selection already painted
	selStart     int      // the selected item's first line …
	selEnd       int      // … and the line after its last
	top          *int     // the scroll position, kept in the pane's state
	fromTop      bool     // the first item is selected: show the top
	empty        string   // shown when there are no lines
	notes        []string // error lines above the list

	input       string         // the composer frame's content; "" for none
	inputBorder lipgloss.Style // its border, by mode
	inputLabel  string
	popup       []string // completion rows over the composer's top edge

	// preview draws the selected item's context into w×h; nil for none.
	preview      func(w, h int) string
	previewTitle string
}

// paneSplit is whether there is room for a second column (the thread, or
// a pane's Preview): from thread_split_cols on.
func (m *Model) paneSplit() bool { return m.layout() == layoutWide }

// paneWidths are the pane's and the preview's outer widths (0: none).
func (m *Model) paneWidths(w int, preview bool) (int, int) {
	if !preview || !m.paneSplit() {
		return w, 0
	}
	pw := min(threadW, w*45/100)
	return w - pw - 1, pw
}

// paneListW is the width a pane builds its lines at.
func (m *Model) paneListW(w int, preview bool) int {
	cw, _ := m.paneWidths(w, preview)
	return cw - 2
}

func (m *Model) renderPane(p paneSpec, w, h int) string {
	cw, pw := m.paneWidths(w, p.preview != nil)
	var composer string
	listH := h
	if p.input != "" {
		composer = frameWith(p.input, cw, lineCount(p.input)+2, p.inputLabel, "", p.inputBorder)
		listH = max(3, h-lineCount(composer))
	}
	bodyH := listH - 2
	inner := cw - 2

	view := append([]string(nil), p.notes...)
	room := max(1, bodyH-len(view))
	if len(p.lines) == 0 {
		view = append(view, strings.Split(placeholder(p.empty, inner, room), "\n")...)
	} else {
		top := 0
		if p.top != nil {
			top = *p.top
		}
		if p.selStart < top {
			top = p.selStart
		}
		if p.selEnd > top+room {
			top = max(p.selStart, p.selEnd-room)
		}
		if p.fromTop {
			top = 0
		}
		top = min(max(top, 0), max(0, len(p.lines)-room))
		if p.top != nil {
			*p.top = top
		}
		view = append(view, p.lines[top:min(len(p.lines), top+room)]...)
	}
	truncateRows(view, inner)
	col := RenderFrame(strings.Join(view, "\n"), cw, listH, p.title, p.right, true)
	if composer != "" {
		above := col
		if len(p.popup) > 0 {
			lines := strings.Split(above, "\n")
			at := max(0, len(lines)-len(p.popup))
			popW := ansi.StringWidth(p.popup[0])
			for k, r := range p.popup {
				if i := at + k; i < len(lines) {
					lines[i] = r + ansi.TruncateLeft(lines[i], popW, "")
				}
			}
			above = strings.Join(lines, "\n")
		}
		col = above + "\n" + composer
	}
	if pw == 0 {
		return col
	}
	pv := strings.Split(p.preview(pw-2, h-2), "\n")
	truncateRows(pv, pw-2)
	preview := RenderFrame(strings.Join(pv, "\n"), pw, h, frameTitle("Preview", false, p.previewTitle), "", false)
	return lipgloss.JoinHorizontal(lipgloss.Top, fit(col, cw, h), gapCol(h), preview)
}

// paneInput is a filter or query in the composer frame: the input while
// typing, otherwise its text in faint.
func paneInput(view, value string, typing bool) string {
	if typing {
		return " " + view
	}
	return " " + sty.Faint.Render(value)
}

// group is a list's faint bold header line: a conversation as it is
// written, a kind of item in caps (BOOKMARKS).
func group(name string, meta string) string {
	h := " " + sty.Header.Render(name)
	if meta != "" {
		h += "  " + sty.Faint.Render(meta)
	}
	return h
}

// blockRows is a pane item made into rows: the bar column, the lines,
// selected or not.
func blockRows(lines []string, w int, sel, focused bool) []string {
	return selLines(prefixAll(lines, " "), w, sel, focused)
}

// previewMessage is a message drawn whole for a preview.
func (m *Model) previewMessage(lines []string, w, h int) string {
	return strings.Join(lines[:min(len(lines), h)], "\n")
}

// fileLine is an attachment's line (DESIGN.md §10): the file type's icon
// (accent for images), the name, then its kind and size in faint. With
// unicode icons the kind is a tag before the name.
func fileLine(f backend.File, size int) string {
	name, mimetype := fileLabel(f), f.Mimetype
	kind := fileKind(f.Name, mimetype)
	if ico.Unicode {
		line := sty.Faint.Render(ico.File) + " " + kindTag(kind) + " " + sty.Normal.Render(name)
		if size > 0 {
			line += "  " + sty.Faint.Render(fmtSize(size))
		}
		return line
	}
	icon := sty.Muted.Render(fileIcon(mimetype, f.Name))
	if strings.HasPrefix(mimetype, "image/") {
		icon = sty.Bar.Render(fileIcon(mimetype, f.Name))
	}
	meta := kind
	if size > 0 {
		meta += " · " + fmtSize(size)
	}
	return icon + " " + sty.Normal.Render(name) + "  " + sty.Faint.Render(meta)
}
