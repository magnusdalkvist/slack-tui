package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/kitty"
)

func TestFitCells(t *testing.T) {
	for _, tc := range []struct{ w, h, cols, rows int }{
		{1600, 900, 40, 10}, // wide screenshot: width-limited
		{900, 1600, 15, 12}, // tall photo: height-limited
		{64, 64, 8, 4},      // small icon: not enlarged past its own width
		{4000, 100, 40, 1},  // banner
	} {
		c, r := fitCells(tc.w, tc.h, 8, 18, 40, 12)
		if c != tc.cols || r != tc.rows {
			t.Errorf("fitCells(%d,%d) = %dx%d, want %dx%d", tc.w, tc.h, c, r, tc.cols, tc.rows)
		}
	}
}

func TestImageInMessage(t *testing.T) {
	m := fakeModel(t, 100, 30)
	f, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	m.SetGraphics(kitty.NewWriter(f, true), "")
	m.open("C2")
	shot := backend.File{ID: "F1", Name: "shot.png", Mimetype: "image/png", Thumb: "https://files.slack.com/t.png"}
	m.update(historyMsg{id: "C2", msgs: []backend.Message{{TS: "100.000001", User: "U1", Text: "look", Files: []backend.File{shot}}}})

	// The model asks for the image once and shows it as loading.
	m.scanImages()
	if st := m.images["F1"]; st == nil || st.status != imgLoading {
		t.Fatalf("image not requested: %+v", st)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "shot.png  loading…") || !strings.Contains(v, "╭┄┄") {
		t.Error("no loading label or reserved frame")
	}
	// Once ready, its placeholders are in the view.
	m.update(imageMsg{fileID: "F1", cols: 10, rows: 3, png: []byte("x")})
	if n := strings.Count(m.View(), "\U0010EEEE"); n != 30 {
		t.Errorf("view has %d placeholder cells, want 10x3", n)
	}
	// Focus resends it through the tmux wrapper.
	if cmd := m.resendImages(); cmd != nil {
		cmd()
	}
	b, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(b), "\x1bPtmux;\x1b\x1b_Ga=T,U=1,f=100") {
		t.Errorf("resend wrote %q", b)
	}
	// A failed download says why instead of breaking the layout.
	m.update(imageMsg{fileID: "F1", err: errors.New("403 Forbidden")})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "403 Forbidden") {
		t.Error("no error label")
	}
}

// TestImageKeepsItsPlace: the frame kept while an image loads is the size
// it arrives at, so nothing below it moves; r on a failed one retries.
func TestImageKeepsItsPlace(t *testing.T) {
	m := fakeModel(t, 120, 40)
	tty, _ := os.CreateTemp(t.TempDir(), "tty")
	m.SetGraphics(kitty.NewWriter(tty, false), "")
	m.cellW, m.cellH = 8, 16
	m.open("C2")
	shot := backend.File{ID: "F1", Name: "shot.png", Mimetype: "image/png", Thumb: "https://files.slack.com/t.png", W: 640, H: 320}
	m.update(historyMsg{id: "C2", msgs: []backend.Message{
		{TS: "100.000001", User: "U1", Text: "look", Files: []backend.File{shot}},
		{TS: "100.000002", User: "U2", Text: "after the picture"},
	}})
	m.scanImages()
	row := func() int {
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "after the picture") {
				return i
			}
		}
		return -1
	}
	before := row()
	cols, rows := m.imageBox(shot)
	m.update(imageMsg{fileID: "F1", cols: cols, rows: rows, png: []byte("x")})
	if after := row(); after != before || before < 0 {
		t.Errorf("the next message moved from row %d to %d when the image loaded", before, after)
	}

	m.update(imageMsg{fileID: "F1", err: errors.New("403")})
	m.focus = fMessages
	m.lists["C2"].sel = 0
	if !strings.Contains(m.renderStatus(), "retry") {
		t.Error("a failed image should offer r retry")
	}
	m.handleKey(key("r"))
	if m.images["F1"] != nil {
		t.Error("r should drop the failed image so it loads again")
	}
}
