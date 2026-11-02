package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

// These never run the returned commands, so nothing is really opened.
func TestOpenChooser(t *testing.T) {
	m := chanWithMessages(t)
	m.update(historyMsg{id: "C2", msgs: []backend.Message{
		{TS: "100.000001", User: "U1", Text: "one <https://a.dk/x|a.dk/x>"},
		{TS: "100.000002", User: "U1", Text: "see <https://docs.dk|the docs> and <https://b.dk>",
			Files: []backend.File{{ID: "F1", Name: "plan.pdf", Mimetype: "application/pdf"}}},
	}})
	m.focus = fMessages

	m.Update(key("o")) // newest message: a file and two links
	if m.opener == nil || len(m.opener.items) != 3 {
		t.Fatalf("want a chooser with 3 items, got %+v", m.opener)
	}
	v := dump(t, m)
	for _, want := range []string{`1 +PDF +plan\.pdf`, `2 +URL +the docs https://docs\.dk`, `3 +URL +https://b\.dk`} {
		if !regexp.MustCompile(want).MatchString(v) {
			t.Errorf("chooser missing %q", want)
		}
	}
	m.Update(key("j"))
	m.Update(key("j"))
	m.Update(key("enter"))
	if m.opener != nil || !strings.Contains(m.status, "opening https://b.dk") {
		t.Errorf("enter should open the third item: opener=%v status=%q", m.opener, m.status)
	}

	m.Update(key("o"))
	m.Update(key("2"))
	if m.opener != nil || !strings.Contains(m.status, "https://docs.dk") {
		t.Errorf("2 should open the docs link: status=%q", m.status)
	}

	m.Update(key("k")) // older message: a single link opens straight away
	m.Update(key("o"))
	if m.opener != nil || !strings.Contains(m.status, "opening https://a.dk/x") {
		t.Errorf("a lone link should open without a chooser: status=%q", m.status)
	}
}

// TestOpenLinkWithPreview: a link and Slack's preview of it are one page;
// o opens it straight away instead of asking between the two. A link only
// in an attachment (a bot's) is still offered.
func TestOpenLinkWithPreview(t *testing.T) {
	m := chanWithMessages(t)
	m.update(historyMsg{id: "C2", msgs: []backend.Message{
		{TS: "100.000003", User: "U1", Text: "read <https://news.dk/a?id=1>", Attachments: []backend.Attachment{{
			FromURL: "https://news.dk/a?id=1", Title: "A story", TitleLink: "https://www.news.dk/a?id=1/", Text: "about things",
		}}},
	}})
	m.focus = fMessages
	m.Update(key("o"))
	if m.opener != nil || !strings.Contains(m.status, "opening https://news.dk/a?id=1") {
		t.Errorf("want the link opened at once: opener=%v status=%q", m.opener, m.status)
	}

	bot := backend.Message{Text: "deploy done", Attachments: []backend.Attachment{{Title: "build 12", TitleLink: "https://ci.dk/12"}}}
	if ls := messageLinks(bot); len(ls) != 1 || ls[0].URL != "https://ci.dk/12" {
		t.Errorf("a bot's attachment link: %+v", ls)
	}
}
