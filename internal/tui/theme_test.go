package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestThemeContrast(t *testing.T) {
	for _, th := range []Theme{Dark(), Light()} {
		for _, k := range textTokens {
			for _, bg := range []string{"bg", "raised"} {
				if r := contrast(th.hex[k], th.hex[bg]); r < 4.5 {
					t.Errorf("%s: %s on %s is %.2f:1", th.Name, k, bg, r)
				}
			}
		}
	}
}

func TestThemeOverrides(t *testing.T) {
	th, warns := LoadTheme("dark", map[string]string{"accent": "#ff00ff", "ok": "39"})
	if len(warns) != 0 {
		t.Errorf("good overrides warned: %v", warns)
	}
	if th.Accent != lipgloss.Color("#ff00ff") || th.OK != lipgloss.Color("39") {
		t.Errorf("overrides not applied: %v %v", th.Accent, th.OK)
	}
	_, warns = LoadTheme("dark", map[string]string{"nope": "#000000", "fg": "blue", "faint": "#222222"})
	got := strings.Join(warns, "\n")
	for _, want := range []string{`"nope": no such token`, `fg = "blue"`, "faint on bg"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings %q lack %q", got, want)
		}
	}
	if _, warns := LoadTheme("purple", nil); len(warns) != 1 {
		t.Errorf("unknown theme name should warn, got %v", warns)
	}
}

func TestNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	th, _ := LoadTheme("dark", map[string]string{"accent": "#ff00ff"})
	if !th.NoColor || th.Accent != (lipgloss.NoColor{}) {
		t.Errorf("NO_COLOR should drop every colour, got %+v", th.Accent)
	}
}

func TestXterm256Hex(t *testing.T) {
	for n, want := range map[int]string{0: "#000000", 75: "#5fafff", 203: "#ff5f5f", 232: "#080808", 255: "#eeeeee"} {
		if got := xterm256Hex(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}
