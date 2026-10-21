package dashboard_test

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// warm is the dashboard on the busy fixture after ten minutes on screen,
// so the history has a shape, then keys.
func warm(t *testing.T, w, h int, keys ...string) string {
	t.Helper()
	f := newFeed(busyK3s)
	f.vary = true
	m := mount(t, f, w, h)
	m = advance(t, m, f, 10*time.Minute)
	m, _ = press(t, m, keys...)
	return tuitest.Screen(m)
}

// TestWideGolden pins panels 1 to 3 on the busy fixture at 207x58, with
// the focus where the mockup has it (panel 3, first row), on a node row,
// and on the banner. Regenerate with
//
//	go test ./internal/tui/views/dashboard -run Golden -update
func TestWideGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
	}{
		{name: "busy_k3s_207x58", keys: []string{"3", "down"}},
		{name: "busy_k3s_207x58_nodes", keys: []string{"2", "down", "down"}},
		{name: "busy_k3s_207x58_overview"},
		{name: "busy_k3s_207x58_workloads", keys: []string{"4", "down", "down", "down", "down"}},
		{name: "busy_k3s_207x58_events", keys: []string{"5", "down", "down", "down", "down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			screen := warm(t, wideW, wideH, tt.keys...)
			requireSize(t, screen, wideW, wideH)
			tuitest.Golden(t, tt.name, screen+"\n")
		})
	}
}

// BenchmarkWideView renders the 207x58 body on the busy fixture with ten
// minutes of history, uncached: the spec's cold render budget is 4 ms.
func BenchmarkWideView(b *testing.B) {
	f := newFeed(busyK3s)
	f.vary = true
	m := mount(b, f, wideW, wideH)
	m = advance(b, m, f, 10*time.Minute)
	m, _ = press(b, m, "3", "down")
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
