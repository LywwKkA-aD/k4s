package top_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Pod and node names from metrics-server render inert; the row key the
// cursor keeps stays raw.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	ns, pod, node := tuitest.Hostile("shop"), tuitest.Hostile("web-1"), tuitest.Hostile("node-1")
	cases := []struct {
		name    string
		keys    []string
		want    []string
		wantKey string
	}{
		{name: "pods", want: []string{"shop", "web-1"}, wantKey: views.RowKey(ns, pod)},
		{name: "nodes", keys: []string{"n"}, want: []string{"node-1"}, wantKey: views.RowKey("", node)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Metrics(
				[]k8stest.PodMetrics{{Namespace: ns, Name: pod, Containers: []k8stest.ContainerUsage{{Name: "app", CPU: "5m", Memory: "1Mi"}}}},
				[]k8stest.NodeMetrics{{Name: node, CPU: "100m", Memory: "1Gi"}},
			))
			m, _ := tuitest.Drive(t, mounted(t, c, ""), tuitest.Keys(tc.keys...)...)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}
			if got := m.State().Selected; got != tc.wantKey {
				t.Errorf("State().Selected = %q, want the raw %q", got, tc.wantKey)
			}
		})
	}
}
