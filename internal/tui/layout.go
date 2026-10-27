package tui

import "github.com/charmbracelet/lipgloss"

// Breakpoints (DESIGN.md §4): from thread_split_cols on, an open thread
// sits beside the chat; from narrow_cols on, it takes the chat's place;
// below that, stack mode shows one frame at a time.

type layoutMode int

const (
	layoutWide   layoutMode = iota // sidebar │ chat │ thread
	layoutMedium                   // sidebar │ chat, or the thread in its place
	layoutStack                    // conversations → chat → thread
)

func (m *Model) layout() layoutMode {
	switch {
	case m.width < m.cfg.Layout.NarrowCols:
		return layoutStack
	case m.width < m.cfg.Layout.ThreadSplitCols:
		return layoutMedium
	}
	return layoutWide
}

func (m *Model) stacked() bool { return m.layout() == layoutStack }

// threadW is the thread frame's width beside a chat w wide.
const threadW = 56

// renderMain draws what goes right of the sidebar, w×h: a pane, the
// chat, the thread, or both. In stack mode it is "" while the
// conversation list has the window.
func (m *Model) renderMain(w, h int, stack bool) string {
	switch {
	case m.help != nil:
		return m.renderHelp(w, h)
	case m.search != nil && m.search.open:
		return m.renderSearch(w, h)
	case m.pins != nil:
		return m.renderPins(w, h)
	case m.sched != nil:
		return m.renderScheduled(w, h)
	case m.chans != nil:
		return m.renderChans(w, h)
	case m.members != nil:
		return m.renderMembers(w, h)
	case m.shared != nil:
		return m.renderShared(w, h)
	case stack && m.focus == fSidebar:
		return ""
	case m.current == unreadsID:
		return m.renderUnreads(w, h)
	case m.thread != nil && m.layout() == layoutWide:
		tw := min(threadW, w*45/100)
		cw := w - tw - 1
		return lipgloss.JoinHorizontal(lipgloss.Top,
			fit(m.renderConvPane(cw, h), cw, h), gapCol(h), fit(m.renderThreadPane(tw, h), tw, h))
	case m.thread != nil && m.focus == fThread:
		return m.renderThreadPane(w, h)
	}
	return m.renderConvPane(w, h)
}
