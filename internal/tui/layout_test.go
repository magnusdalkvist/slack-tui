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

func TestStartupScreens(t *testing.T) {
	m := sceneModel(t, 100, 20)
	m.booted, m.conn, m.cacheChecked = false, "connecting", false
	v := ansi.Strip(m.View())
	for _, want := range []string{"Børsen", "cache loaded", "connecting", "· channels & people", "slack-tui"} {
		if !strings.Contains(v, want) {
			t.Errorf("loading screen lacks %q:\n%s", want, v)
		}
	}
	m.cacheChecked = true
	m.conn = "connected"
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✓ connecting") || !strings.Contains(v, "✓ no cache yet") {
		t.Errorf("finished steps should be ticked:\n%s", v)
	}

	m.bootErr = errFake("invalid_auth")
	v = ansi.Strip(m.View())
	for _, want := range []string{"✕ Couldn't start", "invalid_auth", "config.toml", "ERROR", "e edit config", "q quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("error screen lacks %q:\n%s", want, v)
		}
	}
	if m.handleKey(key("e")) == nil {
		t.Error("e should open the config in an editor")
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
