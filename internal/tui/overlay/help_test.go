package overlay_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func helpSections() []overlay.HelpSection {
	return []overlay.HelpSection{
		{Title: "global", Entries: []overlay.HelpEntry{{Keys: "q", Desc: "quit"}, {Keys: "ctrl+c", Desc: "force quit"}}},
		{Title: "empty"},
		{Title: "commands", Entries: []overlay.HelpEntry{{Keys: ":pods", Desc: "po, pod"}}},
	}
}

func TestHelpView(t *testing.T) {
	t.Parallel()

	o := overlay.NewHelp(helpSections())
	got := screen(o)
	for _, want := range []string{"help", "global", "  q       quit", "  ctrl+c  force quit", "commands", "  :pods  po, pod", "? / Esc close"} {
		if !strings.Contains(got, want) {
			t.Errorf("help lacks %q:\n%s", want, got)
		}
	}
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestHelpClosesOnEveryKey(t *testing.T) {
	t.Parallel()

	help := key.NewBinding(key.WithKeys("?"))
	for _, k := range []string{"?", "esc", "x", ":"} {
		o, out := feed(t, overlay.NewHelp(helpSections(), help), tuitest.Press(k))
		if o != nil || len(out) != 0 {
			t.Errorf("%s: open=%v, messages %s", k, o != nil, tuitest.Describe(out))
		}
	}
	if o, _ := feed(t, overlay.NewHelp(helpSections()), tuitest.Paste("x")); o != nil {
		t.Error("a paste left help open")
	}
	if o, _ := feed(t, overlay.NewHelp(helpSections()), tuitest.Resize(80, 24)); o == nil {
		t.Error("a resize closed help")
	}
}
