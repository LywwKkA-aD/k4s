package dashboard

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// TestUnreadableSaysWhy: a source with nothing to show says why in a
// glyph and words, never as a 0.
func TestUnreadableSaysWhy(t *testing.T) {
	t.Parallel()

	f := frame{ds: styles.Dashboard()}
	tests := []struct {
		a    dash.Avail
		want string
	}{
		{dash.Avail{State: dash.StateForbidden}, "— pods forbidden"},
		{dash.Avail{State: dash.StateUnknown}, "◆ waiting for pods"},
		{dash.Avail{State: dash.StateAbsent}, "○ pods not served"},
		{dash.Avail{State: dash.StateNotReady}, "○ pods not ready"},
		{dash.Avail{State: dash.StateUnauthorized}, "— pods: credentials expired"},
		{dash.Avail{State: dash.StateThrottled}, "△ pods throttled"},
		{dash.Avail{State: dash.StateUnsupported}, "— pods not supported by this server"},
		{dash.Avail{State: dash.StateStale, Stale: 30 * time.Second, Unanswered: true}, "○ pods: no answer yet · stale <1m"},
		{dash.Avail{State: dash.StateFailed}, "— pods failed"},
	}
	for _, tt := range tests {
		t.Run(tt.a.State.String(), func(t *testing.T) {
			t.Parallel()
			if got := ansi.Strip(f.unreadable("pods", tt.a)); got != tt.want {
				t.Errorf("unreadable(%v) = %q, want %q", tt.a.State, got, tt.want)
			}
		})
	}
}
