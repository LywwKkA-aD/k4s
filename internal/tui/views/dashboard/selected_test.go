package dashboard_test

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

var _ views.Selection = dashboard.Model{}

// TestSelectedIsTheObjectUnderTheCursor: o opens the YAML of the object a
// row stands for: a pod, wherever it is listed, or an object k4s
// describes. The banner, a panel, the node chart, a summary line and a
// pod whose namespace is not known yet have none.
func TestSelectedIsTheObjectUnderTheCursor(t *testing.T) {
	t.Parallel()

	xk2lp := views.ResourceRef{Kind: "pod", Namespace: "shop", Name: "payments-worker-6c9f7d8b5-xk2lp"}
	tests := []struct {
		name  string
		base  func() dash.Snapshot
		keys  []string
		panel string
		cmd   string
		want  views.ResourceRef
		ok    bool
	}{
		{name: "the banner", base: busyK3s},
		{name: "a panel", base: busyK3s, keys: []string{"3"}},
		{name: "a pod row", base: busyK3s, keys: []string{"3", "down"}, want: xk2lp, ok: true},
		{name: "a next suggestion about a pod", base: busyK3s, keys: []string{"down"}, want: xk2lp, ok: true},
		{
			name: "a node row", base: busyK3s, panel: "2", cmd: "kubectl describe node k3s-agent-1",
			want: views.ResourceRef{Kind: "node", Name: "k3s-agent-1"}, ok: true,
		},
		{
			name: "a workload row", base: busyK3s, panel: "4", cmd: "kubectl describe statefulset loki-backend -n monitoring",
			want: views.ResourceRef{Kind: "statefulset", Namespace: "monitoring", Name: "loki-backend"}, ok: true,
		},
		{name: "the node chart", base: busyK3s, panel: "2", cmd: "≈ kubectl top nodes"},
		{name: "a summary line", base: busyK3s, panel: "1", cmd: "kubectl get ns"},
		{name: "a row of no object", base: busyK3s, panel: "6", cmd: "≈ kubectl get helmcharts -A"},
		{name: "a pod not placed yet", base: unplaced, panel: "3", cmd: "kubectl get pods -A --field-selector metadata.name=" + r8vqn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(tt.base), wideW, wideH), tt.keys...)
			if tt.panel != "" {
				m = onRow(t, m, tt.panel, tt.cmd)
			}
			got, ok := m.Selected()
			if ok != tt.ok || got != tt.want {
				t.Errorf("Selected() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestSelectedSkipsAGoneObject: a node that left the cluster stays in the
// ring as gone; o has no object to open there, as Enter has none to
// describe.
func TestSelectedSkipsAGoneObject(t *testing.T) {
	t.Parallel()

	cs, fetch, f := nodesOnly(t)
	opts := append(f.options(), dashboard.WithFetch(fetch))
	m, _ := tuitest.Mount(t, dashboard.New(tuitest.NewFakeClient(t), nil, opts...), wideW, wideH)
	if err := cs.CoreV1().Nodes().Delete(context.Background(), "node-b", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	m = advance(t, m, f, 30*time.Second)
	m, _ = press(t, m, "5", "down")
	if got, ok := m.Selected(); ok {
		t.Fatalf("Selected() on node-b, gone, = %+v", got)
	}
}
