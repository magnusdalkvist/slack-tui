package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

func infoModel(t *testing.T) *Model {
	m := fakeModel(t, 110, 30)
	m.open("C2")
	m.update(historyMsg{id: "C2", msgs: []backend.Message{
		{TS: "100.000001", User: "U1", Text: "lunch?", Edited: true, EditedTS: "100.000050",
			ReplyCount: 2, ReplyUsers: []string{"U2", "ME"}, LatestReply: "100.000090",
			Reactions: []backend.Reaction{
				{Name: "tada", Count: 2, Users: []string{"U2", "ME"}},
				{Name: "eyes", Count: 5, Users: []string{"U2"}}, // history cut it short
			},
			Files: []backend.File{{ID: "F1", Name: "menu.pdf", Mimetype: "application/pdf", Size: 14000}}},
		{TS: "100.000002", User: "ME", Text: "yes"},
	}})
	return m
}

func TestInfoPane(t *testing.T) {
	m := infoModel(t)
	m.update(key("k"))
	m.update(key("K"))
	if m.info == nil || m.info.msg.TS != "100.000001" {
		t.Fatalf("K should open the message's info, got %+v", m.info)
	}
	if !m.info.loading {
		t.Error("a reaction with fewer people than its count should be fetched in full")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Message", "Sent", "Edited", "2 replies", "from you, ", "REACTIONS", "you, ", ":eyes:", "loading…", "FILES", "menu.pdf", "14 KB", "Link", "INFO"} {
		if !strings.Contains(v, want) {
			t.Errorf("info lacks %q:\n%s", want, v)
		}
	}

	m.update(infoReactionsMsg{conv: "C2", ts: "100.000001", reactions: []backend.Reaction{
		{Name: "tada", Count: 2, Users: []string{"U2", "ME"}},
		{Name: "eyes", Count: 5, Users: []string{"U2", "U1", "U3", "U4", "U5"}},
	}})
	if m.info.loading {
		t.Error("still loading after the full list came")
	}
	if got := m.lists["C2"].msgs[0].Reactions[1].Users; len(got) != 5 {
		t.Errorf("full list not stored on the message: %v", got)
	}
	if v := ansi.Strip(m.View()); strings.Contains(v, "more") || strings.Contains(v, "loading…") {
		t.Errorf("all five should be named now:\n%s", v)
	}

	// Rows: thread, tada, eyes, file. enter on tada takes ours away.
	m.update(key("j"))
	m.update(key("enter"))
	if rs := m.lists["C2"].msgs[0].Reactions; rs[0].Count != 1 {
		t.Errorf("enter on tada should remove ours: %+v", rs)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "🎉 1") {
		t.Errorf("pane should follow the change:\n%s", v)
	}
	m.update(key("enter"))
	if rs := m.lists["C2"].msgs[0].Reactions; rs[0].Count != 2 {
		t.Errorf("enter again should add it back: %+v", rs)
	}

	m.update(key("g"))
	m.update(key("enter"))
	if m.info != nil || m.thread == nil || m.thread.ts != "100.000001" {
		t.Errorf("enter on the thread row should open the thread")
	}
}

func TestInfoCommand(t *testing.T) {
	m := infoModel(t)
	m.runCmd(":info")
	if m.info == nil || m.info.msg.TS != "100.000002" {
		t.Fatalf(":info should show the selected (newest) message, got %+v", m.info)
	}
	m.update(key("esc"))
	if m.info != nil {
		t.Error("esc closes")
	}
}
