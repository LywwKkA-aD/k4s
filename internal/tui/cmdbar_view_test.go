package tui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// popupWidth is the width of the popup card's top border on the screen.
func popupWidth(t *testing.T, screen string) int {
	t.Helper()
	for _, l := range strings.Split(screen, "\n") {
		if s := strings.TrimSpace(l); strings.HasPrefix(s, "╭") {
			return ansi.StringWidth(s)
		}
	}
	t.Fatalf("no popup on screen:\n%s", screen)
	return 0
}

// An emptied prompt shows its placeholder in the space the input had
// without the prompt label, so the tail popup narrows to the hint line.
func TestTailPopupWidthFollowsThePlaceholder(t *testing.T) {
	t.Parallel()

	req := views.TailPromptRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}}
	m, _ := tuitest.Drive(t, started(t, nil), req)
	if got := popupWidth(t, tuitest.Screen(m)); got != 61 {
		t.Errorf("popup with a value is %d wide, want 61", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("ctrl+u"))
	requireScreen(t, m, "tail lines: 100")
	if got := popupWidth(t, tuitest.Screen(m)); got != 48 {
		t.Errorf("popup showing the placeholder is %d wide, want 48", got)
	}
}
