package tui

import "testing"

func TestCells(t *testing.T) {
	for _, c := range []struct {
		s        string
		from, to int
		want     string
	}{
		{"abcdef", 1, 4, "bcd"},
		{"a🚀b", 0, 2, "a "}, // the rocket's half
		{"a🚀b", 2, 4, " b"},
		{"❤️x", 0, 3, "❤️x"}, // two cells with its variation selector
		{"❤️x", 1, 3, " x"},
	} {
		if got := cells(c.s, c.from, c.to); got != c.want {
			t.Errorf("cells(%q, %d, %d) = %q, want %q", c.s, c.from, c.to, got, c.want)
		}
	}
}
