package overlay_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func helpPage() overlay.HelpPage {
	return overlay.HelpPage{Sections: []overlay.HelpSection{
		{Title: "global", Entries: []overlay.HelpEntry{{Keys: "q", Desc: "quit"}, {Keys: "ctrl+c", Desc: "force quit"}}},
		{Title: "empty"},
		{Title: "commands", Entries: []overlay.HelpEntry{{Keys: ":pods", Desc: "po, pod"}}},
	}}
}

// TestHelpView: the title row says how to close the help, so a popup cut
// at the bottom still says it; sections follow, a blank row between them
// unless the page is tight; an inline section is one row, its title then
// its entries; a section without a title has no title row.
func TestHelpView(t *testing.T) {
	t.Parallel()

	inline := overlay.HelpPage{Sections: []overlay.HelpSection{
		{Title: "global", Inline: true, Entries: []overlay.HelpEntry{{Keys: "q", Desc: "quit"}, {Keys: "^c", Desc: "force quit"}}},
		{Inline: true, Entries: []overlay.HelpEntry{{Keys: ":", Desc: "pods ns deploy"}}},
	}}
	tight := helpPage()
	tight.Tight = true
	cases := []struct {
		name string
		page overlay.HelpPage
		want []string
	}{
		{
			name: "spaced",
			page: helpPage(),
			want: []string{"help   ? / Esc close", "", "global", "  q       quit", "  ctrl+c  force quit", "", "empty", "", "commands", "  :pods  po, pod"},
		},
		{
			name: "tight",
			page: tight,
			want: []string{"help   ? / Esc close", "global", "  q       quit", "  ctrl+c  force quit", "empty", "commands", "  :pods  po, pod"},
		},
		{
			name: "inline",
			page: inline,
			want: []string{"help   ? / Esc close", "", "global  q quit  ^c force quit", "", ": pods ns deploy"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := overlay.NewHelp(tc.page)
			if got, want := screen(o), strings.Join(tc.want, "\n"); got != want {
				t.Errorf("help:\n%s\nwant:\n%s", got, want)
			}
			for width := 1; width <= 80; width++ {
				requireNarrow(t, o, width)
			}
		})
	}
}

func TestHelpClosesOnEveryKey(t *testing.T) {
	t.Parallel()

	help := key.NewBinding(key.WithKeys("?"))
	for _, k := range []string{"?", "esc", "x", ":"} {
		o, out := feed(t, overlay.NewHelp(helpPage(), help), tuitest.Press(k))
		if o != nil || len(out) != 0 {
			t.Errorf("%s: open=%v, messages %s", k, o != nil, tuitest.Describe(out))
		}
	}
	if o, _ := feed(t, overlay.NewHelp(helpPage()), tuitest.Paste("x")); o != nil {
		t.Error("a paste left help open")
	}
	if o, _ := feed(t, overlay.NewHelp(helpPage()), tuitest.Resize(80, 24)); o == nil {
		t.Error("a resize closed help")
	}
}
