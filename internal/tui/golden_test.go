package tui

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/magnusdalkvist/slack-tui/internal/auth"
	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/config"
)

// Golden snapshots of whole screens, for every theme and a few widths.
//
//	go test ./internal/tui -run Golden -update    rewrite testdata/golden
//	SLACKTUI_PREVIEW=/tmp/p go test ./internal/tui -run Golden
//	                                              also write .ansi files to cat in a terminal
var update = flag.Bool("update", false, "rewrite the golden files")

// goldenNow is the pinned clock: Thursday 22 October 2026, 10:45 UTC.
var goldenNow = time.Date(2026, 10, 22, 10, 45, 0, 0, time.UTC)

func ts(day, hour, min int) string {
	t := time.Date(2026, 10, day, hour, min, 0, 0, time.UTC)
	return fmt.Sprintf("%d.000100", t.Unix())
}

// sceneModel is a workspace with a bit of everything in it.
func sceneModel(t *testing.T, w, h int) *Model {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	c := backend.New(auth.Tokens{})
	c.SelfID, c.TeamName = "ME", "Børsen"
	cfg := config.Default()
	cfg.SidebarWidth = 33
	cfg.Sections = []config.Section{{Name: "Team", Items: []string{"#development", "@siko", "@kiky"}}}
	m := New(context.Background(), c, cfg)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.conn = "connected"
	m.Update(bootMsg{
		users: []backend.User{
			{ID: "ME", Name: "mada", DisplayName: "mada", RealName: "Magnus Dalkvist"},
			{ID: "U1", Name: "siko", RealName: "Simon Konstantyner", StatusEmoji: ":house_with_garden:", StatusText: "Working remotely"},
			{ID: "U2", Name: "kim.kyhlensoe", RealName: "Kim Kyhlensø", DisplayName: "kiky"},
			{ID: "U3", Name: "anna", RealName: "Anna Jensen"},
		},
		convs: []backend.Conversation{
			{ID: "C1", Name: "development", Kind: backend.KindChannel, Topic: "Builds, deploys and *reviews*", LastRead: ts(22, 9, 0), Latest: ts(22, 10, 40)},
			{ID: "C2", Name: "general", Kind: backend.KindChannel, LastRead: ts(21, 8, 0), Latest: ts(21, 8, 0)},
			{ID: "C3", Name: "panda-v2", Kind: backend.KindPrivate, LastRead: ts(20, 8, 0), Latest: ts(22, 8, 0), Mentions: 1},
			{ID: "C4", Name: "sjov-og-spass", Kind: backend.KindChannel, LastRead: ts(20, 8, 0), Latest: ts(22, 7, 0)},
			{ID: "D1", Name: "U1", UserID: "U1", Kind: backend.KindDM, LastRead: ts(22, 9, 0), Latest: ts(22, 10, 30), Mentions: 2},
			{ID: "D2", Name: "U2", UserID: "U2", Kind: backend.KindDM, LastRead: ts(22, 9, 0), Latest: ts(22, 9, 0)},
			{ID: "D3", Name: "ME", UserID: "ME", Kind: backend.KindDM, LastRead: ts(21, 9, 0), Latest: ts(21, 9, 0)},
			{ID: "G1", Name: "mpdm-mada--siko--anna-1", Kind: backend.KindGroup, LastRead: ts(19, 9, 0), Latest: ts(19, 9, 0)},
		},
	})
	m.presence["U1"] = true
	m.open("C1")
	m.Update(historyMsg{id: "C1", msgs: []backend.Message{
		{TS: ts(21, 16, 2), User: "U2", Text: "anyone up for reviewing the cache change?"},
		{TS: ts(21, 16, 3), User: "U2", Text: "it's small, promise", Edited: true},
		{TS: ts(22, 9, 12), User: "U1", Text: "deployed `panda-v2` to staging :rocket:", Pinned: true,
			Reactions: []backend.Reaction{{Name: "tada", Count: 2, Users: []string{"ME", "U2"}}, {Name: "eyes", Count: 1, Users: []string{"U3"}}}},
		{TS: ts(22, 9, 40), User: "U1", SubType: "channel_join", Text: "<@U3> has joined the channel"},
		{TS: ts(22, 10, 2), User: "U3", Text: "<@ME> can you look at the failing test? see https://example.com/ci/123",
			ReplyCount: 4, LatestReply: ts(22, 10, 30)},
		{TS: ts(22, 10, 20), BotName: "GitHub", BotID: "B1", Text: "build #812 passed"},
		{TS: ts(22, 10, 40), User: "U1", Text: "plan attached", Files: []backend.File{
			{ID: "F1", Name: "plan.pdf", Mimetype: "application/pdf"},
			{ID: "F2", Name: "diagram.png", Mimetype: "image/png", Thumb: "https://files.slack.com/t.png", W: 800, H: 400},
		}},
	}})
	return m
}

type scene struct {
	name string
	h    int
	prep func(m *Model)
}

// openSceneThread opens the thread under Anna's question.
func openSceneThread(m *Model) {
	l := m.lists["C1"]
	for i, msg := range l.msgs {
		if msg.ReplyCount > 0 {
			l.sel = i
			m.openThread(msg)
			m.Update(repliesMsg{id: "C1", ts: msg.TS, msgs: []backend.Message{
				msg,
				{TS: ts(22, 10, 5), ThreadTS: msg.TS, User: "U1", Text: "on it, looks like a flaky timeout"},
				{TS: ts(22, 10, 30), ThreadTS: msg.TS, User: "ME", Text: "bumped it to 30s :+1:"},
			}})
			return
		}
	}
}

var scenes = []scene{
	{"main", 30, func(m *Model) {}},
	{"sidebar", 30, func(m *Model) { m.focus = fSidebar }},
	{"folded", 30, func(m *Model) {
		// Channels holds panda-v2's mention; Direct messages nothing new.
		for _, name := range []string{"Channels", "Direct messages"} {
			m.sideFindKey("section:" + name)
			m.toggleFold()
		}
		m.focus = fSidebar
	}},
	{"thread", 30, openSceneThread},
	{"switcher", 30, func(m *Model) {
		m.Update(key("ctrl+k"))
		typeText(m, "si")
	}},
	{"insert", 30, func(m *Model) {
		m.Update(key("i"))
		typeText(m, "on it")
	}},
}

var goldenThemes = []struct {
	name    string
	theme   func() Theme
	profile termenv.Profile
	icons   string
}{
	{"dark", Dark, termenv.TrueColor, "nerd"},
	{"light", Light, termenv.TrueColor, "nerd"},
	{"nocolor", noColor, termenv.ANSI, "nerd"},
	{"unicode", Dark, termenv.TrueColor, "unicode"},
}

func TestGolden(t *testing.T) {
	preview := os.Getenv("SLACKTUI_PREVIEW")
	prof, local, clock := lipgloss.ColorProfile(), time.Local, clockNow
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prof)
		time.Local, clockNow = local, clock
		useTheme(Dark())
		useIcons("nerd")
	})
	time.Local = time.UTC
	clockNow = func() time.Time { return goldenNow }

	for _, sc := range scenes {
		for _, th := range goldenThemes {
			for _, w := range []int{80, 120, 160} {
				name := fmt.Sprintf("%s-%s-%d", sc.name, th.name, w)
				t.Run(name, func(t *testing.T) {
					lipgloss.SetColorProfile(th.profile)
					m := sceneModel(t, w, sc.h)
					useTheme(th.theme())
					useIcons(th.icons)
					sc.prep(m)
					got := m.View()
					checkSize(t, got, w, sc.h)
					if preview != "" {
						_ = os.MkdirAll(preview, 0o755)
						_ = os.WriteFile(filepath.Join(preview, name+".ansi"), []byte(got+"\x1b[0m\n"), 0o644)
					}
					compareGolden(t, filepath.Join("testdata", "golden", name+".golden"), got)
				})
			}
		}
	}
}

func checkSize(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Errorf("%d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if lw := ansi.StringWidth(l); lw > w {
			t.Errorf("line %d is %d wide (> %d): %q", i, lw, w, ansi.Strip(l))
		}
	}
}

func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if string(want) != got {
		t.Errorf("view differs from %s (run with -update after checking):\n--- got\n%s\n--- want\n%s",
			path, ansi.Strip(got), ansi.Strip(string(want)))
	}
}

// TestOneFocusedFrame: the focused frame is the only one with an accent
// border, whichever frame has focus.
func TestOneFocusedFrame(t *testing.T) {
	prof := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prof); useTheme(Dark()) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	for _, sc := range []struct {
		name string
		prep func(m *Model)
	}{
		{"chat", func(m *Model) {}},
		{"sidebar", func(m *Model) { m.focus = fSidebar }},
		{"thread", openSceneThread},
		{"thread, chat focused", func(m *Model) { openSceneThread(m); m.focus = fMessages }},
		{"insert", func(m *Model) { m.Update(key("i")) }},
	} {
		m := sceneModel(t, 160, 30)
		useTheme(Dark())
		sc.prep(m)
		corner := sgrPrefix(sty.Bar) + "╭"
		if n := strings.Count(m.View(), corner); n != 1 {
			t.Errorf("%s: %d accent frames, want 1", sc.name, n)
		}
	}
}
