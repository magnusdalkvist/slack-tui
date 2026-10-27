package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
)

func TestHintGrammar(t *testing.T) {
	hs := []Hint{{"a", "one"}, {"b", "two"}, {"c", "three"}, {"d", "four"}, {"e", "five"}, {"f", "six"}}
	got := ansi.Strip(renderHints(hs, 200, true))
	if got != "a one   b two   c three   d four   ? help" {
		t.Errorf("five groups, help last: %q", got)
	}
	if got := ansi.Strip(renderHints(hs, 200, false)); got != "a one   b two   c three   d four   e five" {
		t.Errorf("without help: %q", got)
	}
	// Narrow: real hints first, help only when it still fits.
	if got := ansi.Strip(renderHints([]Hint{{"R", "retry now"}, {"i", "write"}}, 12, true)); got != "R retry now" {
		t.Errorf("narrow: %q", got)
	}
}

func TestOfflineRetry(t *testing.T) {
	m := fakeModel(t, 120, 24)
	m.offline = true
	if !strings.Contains(m.renderStatus(), "R") || !m.isOffline() {
		t.Fatal("offline bar should offer R")
	}
	if cmd := m.handleKey(key("R")); cmd == nil {
		t.Error("R should retry the start")
	}
	m.Update(bootMsg{users: []backend.User{{ID: "ME", Name: "erik"}}})
	if m.offline {
		t.Error("a good start ends offline")
	}
}
