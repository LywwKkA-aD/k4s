package tuitest_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestHostileCleansBackToTheName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"web-1", "shop", "a", ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := tuitest.Hostile(name)
			if got := sanitize.Line(h); got != name {
				t.Errorf("sanitize.Line(Hostile(%q)) = %q", name, got)
			}
			if got := sanitize.Cell(h); got != name {
				t.Errorf("sanitize.Cell(Hostile(%q)) = %q", name, got)
			}
			for _, mark := range []string{"\x1b]52;", "\a", "\x1b[2J", "\u202e", "\u200b", "\U000E0041"} {
				if !strings.Contains(h, mark) {
					t.Errorf("Hostile(%q) lacks %q", name, mark)
				}
			}
		})
	}
}

func TestAssertInertPassesStyledText(t *testing.T) {
	t.Parallel()

	tuitest.AssertInert(t, "\x1b[1;31mweb-1\x1b[0m plain")
}
