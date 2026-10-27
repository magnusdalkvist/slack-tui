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
	"github.com/magnusdalkvist/slack-tui/internal/kitty"
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
	m.conn = "connected"
	m.infoPending, m.infoTotal = 0, 0 // the unread checks are done
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

// withGraphics turns inline images on, drawing into a scratch file.
func withGraphics(m *Model) {
	tty, _ := os.CreateTemp(os.Getenv("XDG_CACHE_HOME"), "tty")
	m.SetGraphics(kitty.NewWriter(tty, false), "")
	m.cellW, m.cellH = 8, 16
	m.imgIDBase = 0x100000 // image ids are random otherwise, and colour the placeholders
}

// lastFile is the scene's image: diagram.png on the last message.
func lastFile(m *Model) backend.File {
	l := m.lists["C1"]
	return l.msgs[len(l.msgs)-1].Files[1]
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
				{TS: ts(22, 10, 30), ThreadTS: msg.TS, User: "ME", Text: "bumped it to 30s :+1:\n```ctx, cancel := context.WithTimeout(ctx, 30*time.Second)```"},
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
	{"complete", 30, func(m *Model) {
		m.Update(key("i"))
		typeText(m, "hey @si")
	}},
	{"attach", 30, func(m *Model) {
		m.Update(key("i"))
		m.attach = []*attachment{{name: "screenshot.png", data: make([]byte, 3000)}, {name: "pasted-2.png", data: make([]byte, 9000)}}
		typeText(m, "here are the screenshots")
	}},
	{"broadcast", 30, func(m *Model) {
		openSceneThread(m)
		m.Update(key("i"))
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		typeText(m, "fixed")
	}},
	{"tall", 30, func(m *Model) {
		var b strings.Builder
		for i := 1; i <= 40; i++ {
			fmt.Fprintf(&b, "log line %02d\n", i)
		}
		l := m.lists["C1"]
		l.msgs = append(l.msgs, backend.Message{TS: ts(22, 10, 44), User: "U2", Text: b.String()})
		l.sel = len(l.msgs) - 1
		m.Update(key("k"))
		m.Update(key("j"))
	}},
	{"react", 30, func(m *Model) {
		m.emoji["partyparrot"] = "https://emoji.slack-edge.com/T/pp/a.gif"
		m.picker = m.newPicker("C1", m.lists["C1"].msgs[2])
	}},
	{"profile", 30, func(m *Model) {
		u := m.users["U1"]
		u.Title, u.Pronouns, u.Email, u.TZ, u.TZLabel, u.TZOffset = "Backend developer", "he/him", "siko@example.com", "Europe/London", "GMT", 0
		m.users["U1"] = u
		m.openProfile("U1")
		m.profile.loading = false
	}},
	{"open", 30, func(m *Model) {
		msg := m.lists["C1"].msgs[len(m.lists["C1"].msgs)-1]
		msg.Files[0].Size, msg.Files[1].Size = 1258291, 219136
		msg.Text = "plan attached, see <https://example.com/ci/123|the CI run>"
		m.openFrom(msg)
	}},
	{"search", 30, func(m *Model) {
		m.Update(key("/"))
		typeText(m, "deploy")
		m.Update(key("enter"))
		mk := func(conv, t, thread, user, text string) backend.Hit {
			return backend.Hit{Message: backend.Message{TS: t, ThreadTS: thread, User: user, Text: text}, Channel: conv}
		}
		m.Update(searchMsg{seq: m.search.seq, query: "deploy", res: backend.SearchPage{Total: 3, Page: 1, Pages: 1, Hits: []backend.Hit{
			mk("C1", ts(22, 9, 12), "", "U1", backend.HitStart + "deploy" + backend.HitEnd + "ed panda-v2 to staging"),
			mk("C1", ts(20, 14, 0), ts(20, 13, 0), "U2", "who can " + backend.HitStart + "deploy" + backend.HitEnd + " on fridays?"),
			mk("D1", ts(21, 11, 0), "", "U1", "the " + backend.HitStart + "deploy" + backend.HitEnd + " script needs a flag"),
		}}})
	}},
	{"files", 30, func(m *Model) {
		m.Update(key("F"))
		m.Update(convFilesMsg{conv: "C1", res: backend.FilesPage{Total: 2, Page: 1, Pages: 1, Files: []backend.SharedFile{
			{File: backend.File{ID: "F1", Name: "plan.pdf", Mimetype: "application/pdf"}, User: "U1", Created: goldenNow.Add(-time.Hour), Size: 1_258_291, Kind: "PDF"},
			{File: backend.File{ID: "F2", Name: "diagram.png", Mimetype: "image/png"}, User: "U2", Created: goldenNow.Add(-26 * time.Hour), Size: 219_136, Kind: "PNG"},
		}}})
	}},
	{"pins", 30, func(m *Model) {
		m.Update(key("P"))
		m.Update(pinsMsg{conv: "C1",
			bookmarks: []backend.Bookmark{{ID: "B1", Title: "Runbook", Link: "https://example.com/runbook"}},
			pins:      []backend.Pin{{Message: m.lists["C1"].msgs[2], Channel: "C1"}},
		})
	}},
	{"scheduled", 30, func(m *Model) {
		m.scheduled = []backend.Scheduled{
			{ID: "Q1", Channel: "C1", Text: "standup in 5", PostAt: goldenNow.Add(time.Hour)},
			{ID: "Q2", Channel: "D1", Text: "ping me when the build is green", PostAt: goldenNow.Add(3 * time.Hour)},
			{ID: "Q3", Channel: "C1", Text: "weekly notes are up", PostAt: goldenNow.Add(26 * time.Hour)},
		}
		m.runCmd("scheduled")
		m.sched.loading = false
	}},
	{"channels", 30, func(m *Model) {
		m.runCmd("join")
		m.Update(publicChansMsg{chans: []backend.PublicChannel{
			{ID: "C1", Name: "development", Members: 24, IsMember: true, Purpose: "Builds, deploys and reviews"},
			{ID: "C9", Name: "design", Members: 9, Purpose: "Mockups and reviews"},
			{ID: "C8", Name: "random", Members: 120},
		}})
	}},
	{"members", 30, func(m *Model) {
		m.runCmd("members")
		m.update(membersMsg{conv: "C1", ids: []string{"U1", "ME", "U2", "U3"}})
	}},
	{"unreads", 30, func(m *Model) {
		m.open(unreadsID)
		m.Update(sinceMsg{id: "C1", msgs: m.lists["C1"].msgs[2:5]})
		m.Update(sinceMsg{id: "D1", msgs: []backend.Message{{TS: ts(22, 10, 30), User: "U1", Text: "got a minute?"}}})
	}},
	{"help", 30, func(m *Model) {
		m.Update(key("?"))
		typeText(m, "pin")
	}},
	{"image-loading", 34, func(m *Model) { withGraphics(m); m.scanImages() }},
	{"image-loaded", 34, func(m *Model) {
		withGraphics(m)
		m.scanImages()
		f := lastFile(m)
		cols, rows := m.imageBox(f)
		m.Update(imageMsg{fileID: f.ID, cols: cols, rows: rows, png: []byte("x")})
	}},
	{"image-failed", 34, func(m *Model) {
		withGraphics(m)
		m.scanImages()
		m.Update(imageMsg{fileID: lastFile(m).ID, err: fmt.Errorf("403")})
	}},
	{"viewer", 30, func(m *Model) {
		withGraphics(m)
		l := m.lists["C1"]
		f := lastFile(m)
		m.viewer = &viewer{items: []viewItem{{file: f, ts: l.msgs[len(l.msgs)-1].TS}}, list: l, id: 7,
			cache: map[string]*viewImg{f.ID: {cols: 40, rows: 10}}, shown: f.ID, details: true}
		m.viewer.w, m.viewer.h = m.viewBox()
	}},
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
	for _, sc := range scenes {
		runGolden(t, sc.name, sc.h, sc.prep, func(m *Model) string { return m.View() })
	}
}

// statusScenes are the status bar's states (DESIGN.md §8), drawn alone.
var statusScenes = []scene{
	{"live", 30, func(m *Model) {}},
	{"me", 30, func(m *Model) {
		u := m.users["ME"]
		u.StatusEmoji, u.StatusText = ":house_with_garden:", "Working remotely"
		m.users["ME"] = u
		m.dnd.SnoozeEnd = goldenNow.Add(45 * time.Minute)
		m.away = true
	}},
	{"progress", 30, func(m *Model) { m.infoPending, m.infoTotal = 68, 80 }},
	{"result", 30, func(m *Model) { m.setResult("saved to ~/Downloads/plan (1).pdf"); m.savedPath = "/x" }},
	{"error", 30, func(m *Model) { m.setStatus("couldn't save: disk full", true) }},
	{"confirm", 30, func(m *Model) {
		m.confirm = &confirmState{prompt: "Delete this message?", detail: "it goes for everyone", yesLabel: "delete", noLabel: "keep"}
	}},
	{"reconnecting", 30, func(m *Model) { m.conn = "connecting" }},
	{"offline", 30, func(m *Model) { m.offline, m.retryAt = true, goldenNow.Add(24*time.Second) }},
	{"insert", 30, func(m *Model) { m.Update(key("i")) }},
	{"command", 30, func(m *Model) { m.Update(key(":")); typeText(m, "st") }},
	{"jump", 30, func(m *Model) { m.Update(key("ctrl+k")) }},
}

func TestGoldenStatus(t *testing.T) {
	for _, sc := range statusScenes {
		runGolden(t, "status-"+sc.name, sc.h, sc.prep, func(m *Model) string {
			v := m.View()
			return v[strings.LastIndex(v, "\n")+1:]
		})
	}
}

// runGolden draws a scene for every theme and width and compares it with
// its golden file.
func runGolden(t *testing.T, name string, h int, prep func(m *Model), draw func(m *Model) string) {
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

	for _, th := range goldenThemes {
		for _, w := range []int{80, 120, 160} {
			full := fmt.Sprintf("%s-%s-%d", name, th.name, w)
			t.Run(full, func(t *testing.T) {
				lipgloss.SetColorProfile(th.profile)
				m := sceneModel(t, w, h)
				useTheme(th.theme())
				useIcons(th.icons)
				prep(m)
				got := draw(m)
				checkSize(t, got, w, strings.Count(got, "\n")+1)
				if strings.Count(got, "\n") > 0 {
					checkSize(t, got, w, h)
				}
				if preview != "" {
					_ = os.MkdirAll(preview, 0o755)
					_ = os.WriteFile(filepath.Join(preview, full+".ansi"), []byte(got+"\x1b[0m\n"), 0o644)
				}
				compareGolden(t, filepath.Join("testdata", "golden", full+".golden"), got)
			})
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
