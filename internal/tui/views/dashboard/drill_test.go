package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// TestEnterOnAPanelOpensItsView: Enter on a panel opens the k4s list its
// command stands for, in the dashboard's scope: the warnings for panel 5,
// kube-system's deployments for panel 6, top sorted as panel 7 is.
func TestEnterOnAPanelOpensItsView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base func() dash.Snapshot
		keys []string
		want views.OpenViewMsg
		hint string
	}{
		{name: "cluster", base: busyK3s, keys: []string{"1"}, want: views.OpenViewMsg{Name: "namespaces"}, hint: "open :ns"},
		{name: "nodes", base: busyK3s, keys: []string{"2"}, want: views.OpenViewMsg{Name: "nodes"}, hint: "open :nodes"},
		{name: "pods", base: busyK3s, keys: []string{"3"}, want: views.OpenViewMsg{Name: "pods"}, hint: "open :pods"},
		{name: "workloads", base: busyK3s, keys: []string{"4"}, want: views.OpenViewMsg{Name: "deployments"}, hint: "open :deploy"},
		{
			name: "events", base: busyK3s, keys: []string{"5"},
			want: views.OpenViewMsg{Name: "events", State: views.State{Mode: views.ModeWarnings}}, hint: "open :events",
		},
		{name: "k3s", base: busyK3s, keys: []string{"6"}, want: views.OpenViewMsg{Name: "deployments", Namespace: "kube-system"}, hint: "open :deploy"},
		{name: "top pods", base: busyK3s, keys: []string{"7"}, want: views.OpenViewMsg{Name: "top"}, hint: "open :top"},
		{
			name: "top pods by memory", base: busyK3s, keys: []string{"7", "s"},
			want: views.OpenViewMsg{Name: "top", State: views.State{Sort: views.SortMemory}}, hint: "open :top",
		},
		{name: "pods of a scoped user", base: nsScopedEKS, keys: []string{"3"}, want: views.OpenViewMsg{Name: "pods", Namespace: "shop"}, hint: "open :pods"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(tt.base), wideW, wideH), tt.keys...)
			if got := hint(m, "enter"); got != tt.hint {
				t.Errorf("enter hint = %q, want %q", got, tt.hint)
			}
			_, out := press(t, m, "enter")
			if !tuitest.Contains(out, tt.want) {
				t.Fatalf("want %#v in:\n%s", tt.want, tuitest.Describe(out))
			}
		})
	}
}

// onRow focuses panel p and steps down to the row whose command is cmd.
func onRow(t *testing.T, m dashboard.Model, p, cmd string) dashboard.Model {
	t.Helper()
	m, _ = press(t, m, p)
	for range 24 {
		m, _ = press(t, m, "down")
		if command(m) == cmd {
			return m
		}
	}
	t.Fatalf("panel %s has no row with %q:\n%s", p, cmd, tuitest.Screen(m))
	return m
}

// TestEnterOnARowOpensItsView: a row that stands for a list opens it, and
// a row of one object describes it (kubectl describes the kinds the
// built-in describer does not); a row with no k4s view says y copies.
func TestEnterOnARowOpensItsView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, panel, cmd string
		want             any
		hint             string
	}{
		{name: "namespaces", panel: "1", cmd: "kubectl get ns", want: views.OpenViewMsg{Name: "namespaces"}, hint: "open :ns"},
		{
			name: "network", panel: "1", cmd: "kubectl get svc -A --field-selector spec.type=LoadBalancer",
			want: views.OpenViewMsg{Name: "services"}, hint: "open :svc",
		},
		{name: "a node", panel: "2", cmd: "kubectl describe node k3s-agent-1", want: views.DescribeRequestMsg{Kind: "node", Name: "k3s-agent-1"}, hint: "describe"},
		{name: "the node chart", panel: "2", cmd: "≈ kubectl top nodes", want: views.OpenViewMsg{Name: "top", State: views.State{Mode: views.ModeNodes}}, hint: "open :top"},
		{
			name: "a statefulset", panel: "4", cmd: "kubectl describe statefulset loki-backend -n monitoring",
			want: views.DescribeRequestMsg{Kind: "statefulset", Namespace: "monitoring", Name: "loki-backend"}, hint: "describe",
		},
		{
			name: "servicelb", panel: "6", cmd: "kubectl get svc -A --field-selector spec.type=LoadBalancer",
			want: views.OpenViewMsg{Name: "services"}, hint: "open :svc",
		},
		{name: "the helm charts", panel: "6", cmd: "≈ kubectl get helmcharts -A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := onRow(t, mount(t, newFeed(busyK3s), wideW, wideH), tt.panel, tt.cmd)
			if got := hint(m, "enter"); got != tt.hint {
				t.Errorf("enter hint = %q, want %q", got, tt.hint)
			}
			_, out := press(t, m, "enter")
			if tt.want == nil {
				if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "y copies") {
					t.Fatalf("enter on a row with no view:\n%s", tuitest.Describe(out))
				}
				return
			}
			if !tuitest.Contains(out, tt.want) {
				t.Fatalf("want %#v in:\n%s", tt.want, tuitest.Describe(out))
			}
		})
	}
}
