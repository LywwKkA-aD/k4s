package overlay_test

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestPlacement(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Time{} }
	cases := []struct {
		name string
		o    overlay.Overlay
		want overlay.Placement
	}{
		{name: "command bar", o: overlay.NewCommand(), want: overlay.Footer},
		{name: "tail", o: overlay.NewTail(views.TailPromptRequestMsg{Pods: []string{"p"}}), want: overlay.Popup},
		{name: "container", o: overlay.NewContainer(views.ContainerPromptRequestMsg{Containers: []string{"c"}}), want: overlay.Popup},
		{name: "forward", o: overlay.NewForward(views.ForwardRequestMsg{Kind: "pod", Name: "p"}, nil), want: overlay.Popup},
		{name: "restore", o: overlay.NewRestore(1, now), want: overlay.Popup},
		{name: "help", o: overlay.NewHelp(overlay.HelpPage{}), want: overlay.Popup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := overlay.PlacementOf(tc.o); got != tc.want {
				t.Fatalf("PlacementOf = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOnlyHelpPassesInputOn(t *testing.T) {
	t.Parallel()

	help := overlay.NewHelp(overlay.HelpPage{}, key.NewBinding(key.WithKeys("?")), key.NewBinding(key.WithKeys("q")))
	cases := []struct {
		name string
		o    overlay.Overlay
		msg  tea.Msg
		want bool
	}{
		{name: "help passes a colon", o: help, msg: tuitest.Press(":"), want: true},
		{name: "help passes a view key", o: help, msg: tuitest.Press("r"), want: true},
		{name: "help passes a paste", o: help, msg: tuitest.Paste("x"), want: true},
		{name: "help keeps its question mark", o: help, msg: tuitest.Press("?")},
		{name: "help keeps q", o: help, msg: tuitest.Press("q")},
		{name: "help keeps esc", o: help, msg: tuitest.Esc()},
		{name: "help keeps alt+esc", o: help, msg: tuitest.Press("alt+esc")},
		{name: "help ignores other messages", o: help, msg: tuitest.Resize(80, 24)},
		{name: "the command bar passes nothing", o: overlay.NewCommand(), msg: tuitest.Press(":")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := overlay.Passes(tc.o, tc.msg); got != tc.want {
				t.Fatalf("Passes = %v, want %v", got, tc.want)
			}
		})
	}
}
