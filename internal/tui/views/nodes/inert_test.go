package nodes_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Every cell a node reports renders inert, a filter finds the node by the
// name it shows, and the selection keeps the raw name.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	raw := tuitest.Hostile("node-1")
	cases := []struct {
		name string
		keys []string
	}{
		{name: "the list"},
		{name: "filtered by the shown name", keys: []string{"/", "n", "o", "d", "e", "-", "1", "enter"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := node(raw, tuitest.Hostile("10.0.0.5"), tuitest.Hostile("v1.36.5"), tuitest.Hostile("arm64"),
				map[string]string{rolePrefix + tuitest.Hostile("worker"): "true"}, 0)
			m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(n)))
			m, _ = tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)

			tuitest.AssertInert(t, tuitest.Frame(m))
			got := strings.Join(rows(m), "\n")
			for _, w := range []string{"node-1", "worker", "10.0.0.5", "v1.36.5", "linux/arm64"} {
				if !strings.Contains(got, w) {
					t.Errorf("rows lack the cleaned %q:\n%s", w, got)
				}
			}
			if ref, ok := m.Selected(); !ok || ref != (views.ResourceRef{Kind: "node", Name: raw}) {
				t.Errorf("Selected() = %+v, %v, want the raw name", ref, ok)
			}
		})
	}
}
