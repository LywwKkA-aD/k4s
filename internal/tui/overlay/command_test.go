package overlay_test

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestCommandBar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []tea.Msg
		want  []tea.Msg
	}{
		{name: "enter sends the input", input: []tea.Msg{tuitest.Type("pods"), tuitest.Enter()}, want: []tea.Msg{overlay.CommandMsg{Input: "pods"}}},
		{name: "a paste is typed", input: []tea.Msg{tuitest.Paste("svc"), tuitest.Enter()}, want: []tea.Msg{overlay.CommandMsg{Input: "svc"}}},
		{name: "empty enter still answers", input: keys("enter"), want: []tea.Msg{overlay.CommandMsg{}}},
		{name: "esc cancels", input: []tea.Msg{tuitest.Type("pods"), tuitest.Esc()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, out := feed(t, overlay.NewCommand(), tc.input...)
			if o != nil {
				t.Fatalf("still open:\n%s", screen(o))
			}
			if !reflect.DeepEqual(out, tc.want) {
				t.Fatalf("messages = %s, want %v", tuitest.Describe(out), tc.want)
			}
		})
	}
}

func TestCommandBarView(t *testing.T) {
	t.Parallel()

	o := overlay.NewCommand()
	if got := strings.TrimSpace(screen(o)); got != ": pods, ns, dashboard" {
		t.Errorf("empty bar = %q", got)
	}
	o, _ = feed(t, o, tuitest.Type("dep"))
	requireShows(t, o, ": dep")
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestCommandBarIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	o, _ := feed(t, overlay.NewCommand(), tuitest.Resize(80, 24))
	requireShows(t, o, ": pods, ns, dashboard")
}
