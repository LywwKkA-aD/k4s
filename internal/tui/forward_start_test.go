package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestForwardStartCmd: a start runs off the event loop and an error it
// returns reaches the status row, even one the forward's row never shows
// (an unknown or refused forward).
func TestForwardStartCmd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want any
	}{
		{name: "started", want: nil},
		{
			name: "refused",
			err:  errors.New("forward pod/web is saved for another cluster"),
			want: views.StatusMsg{Level: status.Error, Text: "port-forward: forward pod/web is saved for another cluster"},
		},
		{
			name: "an error quoting cluster text",
			err:  errors.New("pod \"a\x1b]0;x\x07b\" not found"),
			want: views.StatusMsg{Level: status.Error, Text: "port-forward: pod \"ab\" not found"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotID string
			start := func(ctx context.Context, id string) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("start ran without a deadline")
				}
				gotID = id
				return tc.err
			}
			if got := forwardStartCmd(start, "f1")(); got != tc.want {
				t.Errorf("msg = %#v, want %#v", got, tc.want)
			}
			if gotID != "f1" {
				t.Errorf("started %q, want f1", gotID)
			}
		})
	}
}
