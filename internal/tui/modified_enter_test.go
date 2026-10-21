package tui_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Terminals with the kitty keyboard protocol (Alacritty, kitty, WezTerm,
// ghostty) send shift+enter and ctrl+enter as their own keys; under v1 they
// arrived as a bare Enter and opened describe like Enter does.
func TestModifiedEnterOnARowOpensDescribe(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"shift+enter", "ctrl+enter"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			_, msgs := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys(k))...)
			if _, ok := tuitest.Find[views.DescribeRequestMsg](msgs); !ok {
				t.Fatalf("%s on a pod row did not request describe; messages: %s", k, tuitest.Describe(msgs))
			}
		})
	}
}
