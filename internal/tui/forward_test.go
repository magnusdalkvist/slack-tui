package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/render"
)

// TestQuoteReply: `>` puts the message in the composer behind "> ", with
// mentions typed the way they read, and the cursor below for the reply.
func TestQuoteReply(t *testing.T) {
	m := chanWithMessages(t)
	m.handleKey(key(">"))
	if !m.insert {
		t.Fatal("> should start writing")
	}
	if got, want := m.input.Value(), "> yes @Anna Jensen & #dev\n"; got != want {
		t.Errorf("composer %q; want %q", got, want)
	}
	m.input.InsertString("agreed")
	if got, want := m.outgoing("C2", m.input.Value()), "&gt; yes <@U1> &amp; <#C1>\nagreed"; got != want {
		t.Errorf("sent %q; want %q", got, want)
	}
}

// TestForward: `f` opens "Forward to" (no Unreads in it); the pick opens
// with the message waiting above the composer, and it goes out as the
// note and then the message's link.
func TestForward(t *testing.T) {
	m := chanWithMessages(t)
	m.client.TeamURL = "https://acme.slack.com/"
	m.lists["C2"].sel = 0
	m.handleKey(key("f"))
	if m.switcher == nil || m.switcher.forward == nil {
		t.Fatal("f did not open Forward to")
	}
	if v := dump(t, m); !strings.Contains(v, "Forward to") {
		t.Error("the picker should say Forward to")
	}
	for _, id := range m.switcher.matches {
		if id == unreadsID {
			t.Error("Unreads is no place to forward to")
		}
	}
	for _, r := range "dev" {
		m.handleKey(key(string(r)))
	}
	m.handleKey(key("enter"))
	if m.current != "C1" || !m.insert {
		t.Fatalf("current %q insert %v; want #dev open for writing", m.current, m.insert)
	}
	if v := dump(t, m); !strings.Contains(v, "↪ Anna Jensen in #general: lunch?") || !strings.Contains(v, "forwarding") {
		t.Error("the composer should show the forwarded message")
	}
	if m.forwardFor("C2") != nil {
		t.Error("the forward belongs to #dev only")
	}
	if got, want := m.outgoing("C1", "look"), "look\nhttps://acme.slack.com/archives/C2/p100000001"; got != want {
		t.Errorf("sent %q; want %q", got, want)
	}
	if m.forward != nil {
		t.Error("sending should use up the forward")
	}
}

// TestForwardToNewDM: picking someone with no DM yet opens one, then its
// composer with the forward.
func TestForwardToNewDM(t *testing.T) {
	m := chanWithMessages(t)
	m.users["U3"] = backend.User{ID: "U3", Name: "carl", RealName: "Carl Berg"}
	m.handleKey(key("f"))
	m.forwardTo(m.switcher.forward, personPrefix+"U3")
	m.switcher = nil
	m.update(dmOpenedMsg{conv: backend.Conversation{ID: "D3", Name: "U3", UserID: "U3", Kind: backend.KindDM, InfoLoaded: true}})
	if m.current != "D3" || !m.insert || m.forwardFor("D3") == nil {
		t.Errorf("current %q insert %v; want the new DM open with the forward", m.current, m.insert)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.forward != nil {
		t.Error("ctrl+x should drop the forward")
	}
}

// TestCtrlKWhileWriting: ctrl+k jumps from insert mode too, keeping the
// draft.
func TestCtrlKWhileWriting(t *testing.T) {
	m := chanWithMessages(t)
	m.handleKey(key("i"))
	m.input.InsertString("half a thought")
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.switcher == nil || m.insert {
		t.Fatalf("switcher %v insert %v; want Jump to open", m.switcher != nil, m.insert)
	}
	if m.input.Value() != "half a thought" {
		t.Errorf("draft %q lost", m.input.Value())
	}
}

// TestForwardPreview: Slack's preview of a forwarded message arrives in a
// change event. It shows after what we wrote and our files, in a box like
// an image's: who wrote it and where, then what it says. It doesn't make our message "edited";
// a changed text does.
func TestForwardPreview(t *testing.T) {
	m := chanWithMessages(t)
	link := "https://acme.slack.com/archives/C2/p100000001"
	m.update(backend.EvMessage{Channel: "C2", Msg: backend.Message{TS: "100.000003", User: "ME", Text: "look\n<" + link + ">",
		Files: []backend.File{{ID: "F1", Name: "notes.txt", Mimetype: "text/plain"}}}})
	m.update(backend.EvMessageChanged{Channel: "C2", Msg: backend.Message{
		TS: "100.000003", User: "ME", Text: "look\n<" + link + ">", Edited: true,
		// As Slack sends it for a message sent through an app: the app's
		// name as the author, the person only in the fallback.
		Attachments: []backend.Attachment{{
			Author: "Slack TUI", Text: "lunch?", Footer: "Slack Conversation", TS: "100.000001", FromURL: link,
			Fallback: "[October 27th, 2026 7:35 AM] anna: lunch?",
		}},
	}})
	msg := m.lists["C2"].msgs[2]
	if msg.Edited {
		t.Error("a preview arriving is no edit")
	}
	v := dump(t, m)
	for _, want := range []string{"│ Anna Jensen ", "│ #general", "│ lunch?"} {
		if !strings.Contains(v, want) {
			t.Errorf("preview lacks %q", want)
		}
	}
	if f, box := strings.Index(v, "notes.txt"), strings.Index(v, "│ Anna Jensen"); f < 0 || box < f {
		t.Error("the preview should come last, after the files")
	}
	if strings.Contains(v, "Slack TUI") || strings.Contains(v, "Slack Conversation") {
		t.Error("the preview should name the person, not the app")
	}
	if strings.Contains(v, "(edited)") {
		t.Error("shown as edited")
	}
	if text, _ := render.Editable(msg.Text, m); strings.Contains(text, "lunch?") {
		t.Error("the preview must stay out of our text (e edits it)")
	}

	msg.Text, msg.Edited = "look at this\n<"+link+">", true
	m.update(backend.EvMessageChanged{Channel: "C2", Msg: msg})
	if !m.lists["C2"].msgs[2].Edited {
		t.Error("a changed text is an edit")
	}
}

// TestBotFields: a bot's attachment shows its fields two to a row, as
// Runscope's run results do, and a bare hex colour reads as pass or fail.
func TestBotFields(t *testing.T) {
	m := chanWithMessages(t)
	m.update(backend.EvMessage{Channel: "C2", Msg: backend.Message{TS: "100.000003", BotID: "B1", BotName: "Runscope",
		Attachments: []backend.Attachment{{
			Color: "ff0000", Pretext: "API test run completed.", Title: "borsen.cloud: Newsletter",
			Fields: []backend.Field{
				{Title: "Status", Value: "Failed", Short: true}, {Title: "Requests Executed", Value: "4", Short: true},
				{Title: "Assertions Passed", Value: "3 of 4", Short: true}, {Title: "Total Response Time", Value: "3711ms", Short: true},
				{Title: "Location", Value: "runscope.borsen.cloud", Short: true},
			},
		}}}})
	v := dump(t, m)
	for _, want := range []string{"┃ Status", "Requests Executed", "┃ Failed", "3 of 4", "┃ runscope.borsen.cloud"} {
		if !strings.Contains(v, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	if !regexp.MustCompile(`Failed +4`).MatchString(v) {
		t.Error("short fields should sit side by side")
	}
}

func TestEdge(t *testing.T) {
	prof := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // so styles emit escapes
	t.Cleanup(func() { lipgloss.SetColorProfile(prof) })
	s := NewStyles(Dark())
	for c, want := range map[string]lipgloss.Style{"danger": s.Err, "ff0000": s.Err, "#a30200": s.Err, "2ecc71": s.OK,
		"good": s.OK, "#daa038": s.Warn, "f00": s.Err} {
		if got, ok := s.Edge(c); !ok || got.Render("x") != want.Render("x") {
			t.Errorf("Edge(%q) = %q, want %q", c, got.Render("x"), want.Render("x"))
		}
	}
	for _, c := range []string{"", "#888888", "nope"} {
		if _, ok := s.Edge(c); ok {
			t.Errorf("Edge(%q) should show no colour", c)
		}
	}
}
