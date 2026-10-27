package tui

import (
	"fmt"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// opener chooses what `o` opens when a message has several files or links.
type opener struct {
	items []openItem
	sel   int
	msg   backend.Message // whose files and links they are
}

type openItem struct {
	file *backend.File // a file, or
	link render.Link   // a web link
}

const openerRows = 9

// openFrom opens a message's only file or link, or lets you choose.
func (m *Model) openFrom(msg backend.Message) tea.Cmd {
	var items []openItem
	for i := range msg.Files {
		items = append(items, openItem{file: &msg.Files[i]})
	}
	for _, l := range render.Links(msg.Text) {
		items = append(items, openItem{link: l})
	}
	switch len(items) {
	case 0:
		return m.setStatus("no file or link on this message", false)
	case 1:
		return m.openItem(items[0])
	}
	m.opener = &opener{items: items, msg: msg}
	return nil
}

func (m *Model) openItem(it openItem) tea.Cmd {
	if f := it.file; f != nil {
		return tea.Batch(m.openFile(*f), m.setStatus("opening "+f.Name+"…", false))
	}
	return tea.Batch(m.openURL(it.link.URL), m.setStatus("opening "+it.link.URL, false))
}

func (m *Model) openerKey(k tea.KeyMsg) tea.Cmd {
	o := m.opener
	switch s := k.String(); s {
	case "esc", "q", "o":
		m.opener = nil
	case "j", "down", "tab":
		o.sel = (o.sel + 1) % len(o.items)
	case "k", "up", "shift+tab":
		o.sel = (o.sel - 1 + len(o.items)) % len(o.items)
	case "enter", "l":
		m.opener = nil
		return m.openItem(o.items[o.sel])
	default:
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			if i := int(s[0] - '1'); i < len(o.items) {
				m.opener = nil
				return m.openItem(o.items[i])
			}
		}
	}
	return nil
}

// renderOpener is the Open modal: a number to press, the kind, the name,
// the size.
func (m *Model) renderOpener() string {
	o := m.opener
	w := m.modalWidth(66)
	inner := w - 2
	var rows []string
	start := max(0, o.sel-openerRows+1)
	for i := start; i < len(o.items) && i < start+openerRows; i++ {
		it := o.items[i]
		num := " "
		if i < 9 {
			num = fmt.Sprint(i + 1)
		}
		var row, meta string
		if f := it.file; f != nil {
			name := f.Title
			if name == "" {
				name = f.Name
			}
			row = kindCol(fileKind(f.Name, f.Mimetype)) + " " + sty.Normal.Render(name)
			if f.Size > 0 {
				meta = sty.Faint.Render(fmtSize(f.Size))
			}
		} else {
			row = kindCol("URL") + " "
			if it.link.Label != "" && it.link.Label != it.link.URL {
				row += sty.Normal.Render(it.link.Label) + " "
			}
			row += sty.Md.Link.Render(it.link.URL)
		}
		rows = append(rows, modalRow(rightAlign(" "+sty.Faint.Render(num)+" "+row, meta, inner), inner, i == o.sel))
	}
	right := fmt.Sprintf("%d items · %s %s", len(o.items), m.senderName(o.msg), o.msg.Time().Format(m.cfg.TimeFormat))
	return modalBox(w, "Open", right, rows)
}

// kindTag is a file's kind as a small tag: [PDF].
func kindTag(kind string) string { return sty.KindTag.Render(kind) }

// kindCol is kindTag in a column four letters wide, so names line up.
func kindCol(kind string) string { return kindTag(kind) + strings.Repeat(" ", max(0, 4-len(kind))) }

// fileKind is the extension in caps (PDF, PNG), or from the MIME type.
func fileKind(name, mimetype string) string {
	if ext := strings.TrimPrefix(path.Ext(name), "."); ext != "" && len(ext) <= 5 {
		return strings.ToUpper(ext)
	}
	if _, sub, ok := strings.Cut(mimetype, "/"); ok && len(sub) <= 5 {
		return strings.ToUpper(sub)
	}
	return "FILE"
}
