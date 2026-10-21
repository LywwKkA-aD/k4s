package dashboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func denied(canI ...string) dash.Avail {
	return dash.Avail{State: dash.StateForbidden, CanI: kc(canI...)}
}

// TestDegradedSources renders panels 1 to 3 over sources that are denied,
// absent, stale or empty: each says so in words and a glyph, and a denied
// source never reads as 0.
func TestDegradedSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*dash.Snapshot)
		want   []string
		not    []string
	}{
		{
			name: "nodes forbidden",
			change: func(s *dash.Snapshot) {
				s.Nodes = dash.NodesPanel{Avail: denied("auth", "can-i", "list", "nodes"), Cmd: s.Nodes.Cmd}
			},
			want: []string{"— nodes: list forbidden for devops-friend", "kubectl auth can-i list nodes", "pods per node still shown in 3 pods"},
			not:  []string{"0/0 Ready"},
		},
		{
			name: "no metrics-server on k3s",
			change: func(s *dash.Snapshot) {
				s.Nodes.Metrics = dash.Avail{State: dash.StateAbsent}
				s.Cluster.Version.Distro = "k3s"
			},
			want: []string{
				"○ no metrics-server: metrics.k8s.io is not served",
				"bundled with k3s; --disable=metrics-server?",
			},
			not: []string{"cluster cpu", "re-checked"},
		},
		{
			name: "no metrics-server elsewhere",
			change: func(s *dash.Snapshot) {
				s.Nodes.Metrics = dash.Avail{State: dash.StateAbsent}
				s.Cluster.Version.Distro = "eks"
			},
			want: []string{"○ no metrics-server: metrics.k8s.io is not served", "install: kubernetes-sigs/metrics-server"},
		},
		{
			name:   "metrics not ready",
			change: func(s *dash.Snapshot) { s.Nodes.Metrics = dash.Avail{State: dash.StateNotReady} },
			want:   []string{"○ metrics not ready"},
		},
		{
			name: "metrics forbidden",
			change: func(s *dash.Snapshot) {
				s.Nodes.Metrics = denied("auth", "can-i", "list", "nodes.metrics.k8s.io")
			},
			want: []string{"— metrics forbidden", "kubectl auth can-i list nodes.metrics.k8s.io"},
		},
		{
			name:   "no nodes",
			change: func(s *dash.Snapshot) { s.Nodes.Rows, s.Nodes.Total, s.Nodes.Ready = nil, 0, 0 },
			want:   []string{"○ no nodes registered"},
		},
		{
			name: "a node in trouble",
			change: func(s *dash.Snapshot) {
				s.Nodes.Rows[2].Condition, s.Nodes.Rows[2].Sev = "NotReady", dash.SevFailing
				s.Nodes.Rows[1].Pressures = []string{"MemoryPressure"}
				s.Nodes.Rows[0].Unschedulable = true
			},
			want: []string{"▲ k3s-agent-2", "NotReady", "MemoryPressure", "SchedulingDisabled"},
		},
		{
			name: "pods forbidden",
			change: func(s *dash.Snapshot) {
				s.Pods = dash.PodsPanel{Avail: denied("auth", "can-i", "list", "pods", "-A"), Cmd: s.Pods.Cmd}
			},
			want: []string{"— pods forbidden", "kubectl auth can-i list pods -A"},
			not:  []string{"185"},
		},
		{
			name: "no pods in the namespace",
			change: func(s *dash.Snapshot) {
				s.Pods = dash.PodsPanel{Avail: ok(), Scope: "shop", Cmd: kc("get", "pods", "-n", "shop")}
			},
			want: []string{"○ no pods in shop", "0 · ns shop"},
		},
		{
			name: "nothing needs attention",
			change: func(s *dash.Snapshot) {
				s.Pods.Problems, s.Pods.Restarted = nil, nil
			},
			want: []string{"● nothing needs attention"},
		},
		{
			name: "pods not served",
			change: func(s *dash.Snapshot) {
				s.Pods = dash.PodsPanel{Avail: dash.Avail{State: dash.StateAbsent}}
			},
			want: []string{"○ pods not served"},
		},
		{
			name: "cluster lines denied, stale and empty",
			change: func(s *dash.Snapshot) {
				s.Cluster.Namespaces.Avail = denied("auth", "can-i", "list", "namespaces")
				s.Cluster.Storage = dash.StorageRow{Avail: dash.Avail{State: dash.StateStale, Stale: 2 * time.Minute}}
				s.Cluster.Network.Avail = denied("auth", "can-i", "list", "services", "-A")
				s.Cluster.Version.Avail = denied("auth", "can-i", "get", "/version")
				s.Cluster.API = dash.APIRow{Avail: ok(), Checks: 8, Passed: 6, Failed: []string{"etcd", "informer-sync"}, Etcd: dash.StateFailed}
				s.Cluster.Age.Known = false
			},
			want: []string{
				"— ns", "forbidden · :ns <name> to switch", "○ storage    no pvc · stale 2m",
				"— network    services forbidden", "— version    version forbidden",
				"▲ api        not ready · failing etcd, informer-sync · 6/8 checks",
			},
			not: []string{"(namespace kube-system)"},
		},
		{
			name: "pending and lost claims",
			change: func(s *dash.Snapshot) {
				s.Cluster.Storage = dash.StorageRow{Avail: ok(), Total: 4, Bound: 2, Pending: 1, Lost: 1, Sev: dash.SevWaiting}
			},
			want: []string{"△ storage    2/4 pvc Bound · 1 Pending · 1 Lost"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := func() dash.Snapshot {
				s := busyK3s()
				tt.change(&s)
				return s
			}
			screen := tuitest.Screen(mount(t, newFeed(base), wideW, wideH))
			requireSize(t, screen, wideW, wideH)
			for _, w := range tt.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks %q:\n%s", w, screen)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(screen, n) {
					t.Errorf("screen shows %q:\n%s", n, screen)
				}
			}
		})
	}
}

// TestDeniedRowCarriesItsProbe: the cursor on a denied line puts the auth
// can-i probe in the footer, the command that tells why.
func TestDeniedRowCarriesItsProbe(t *testing.T) {
	t.Parallel()

	base := func() dash.Snapshot {
		s := busyK3s()
		s.Cluster.Storage = dash.StorageRow{Avail: denied("auth", "can-i", "list", "persistentvolumeclaims", "-A")}
		return s
	}
	m, _ := press(t, mount(t, newFeed(base), wideW, wideH), "1", "down", "down", "down", "down", "down", "down")
	if got := command(m); got != "kubectl auth can-i list persistentvolumeclaims -A" {
		t.Fatalf("command on the denied storage line = %q", got)
	}
}
