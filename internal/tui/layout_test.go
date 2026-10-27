package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestStackMode: at 80 columns one frame fills the window, and
// list → ⏎ → chat → t → thread → esc → esc walks down and back up.
func TestStackMode(t *testing.T) {
	m := sceneModel(t, 80, 24)
	m.focus = fSidebar
	m.sideFindKey("C1")
	level := func(want string) {
		t.Helper()
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		top := lines[0]
		if !strings.HasPrefix(top, "╭ "+want) || ansi.StringWidth(top) != 80 || !strings.HasSuffix(top, "╮") {
			t.Fatalf("want the %q frame across the window, top row %q", want, top)
		}
		if strings.Count(top, "╭") != 1 {
			t.Errorf("more than one frame side by side: %q", top)
		}
	}
	level("Børsen")
	m.Update(key("enter"))
	level("‹ " + ico.Channel + " development")
	if !strings.Contains(ansi.Strip(m.View()), "Børsen › development") {
		t.Error("the chat should say where it is: Børsen › development")
	}
	if !strings.Contains(m.renderStatus(), "back") {
		t.Error("the bar should say how to go back")
	}
	// Onto the message with replies, then t.
	l := m.lists["C1"]
	for i, msg := range l.msgs {
		if msg.ReplyCount > 0 {
			l.sel = i
		}
	}
	m.Update(key("t"))
	level("‹ Thread")
	m.Update(key("esc"))
	level("‹ " + ico.Channel + " development")
	m.Update(key("esc"))
	level("Børsen")
}

// TestBreakpoints: three frames from thread_split_cols, the thread in the
// chat's place below it.
func TestBreakpoints(t *testing.T) {
	for _, c := range []struct {
		w      int
		frames int // frames across the top row
		thread bool
	}{{160, 3, true}, {140, 3, true}, {120, 2, true}, {100, 2, true}} {
		m := sceneModel(t, c.w, 30)
		openSceneThread(m)
		top := strings.Split(ansi.Strip(m.View()), "\n")[0]
		if n := strings.Count(top, "╭"); n != c.frames {
			t.Errorf("%d cols: %d frames, want %d: %q", c.w, n, c.frames, top)
		}
		if strings.Contains(top, "Thread") != c.thread {
			t.Errorf("%d cols: thread shown = %v", c.w, !c.thread)
		}
	}
}
