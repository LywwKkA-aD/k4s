package top_test

import (
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// metricsDeniedClusterWide serves metrics.k8s.io to a user whose RBAC
// stops at one namespace: pods across all namespaces are Forbidden, pods
// in demo are served.
func metricsDeniedClusterWide() http.Handler {
	pods := []k8stest.PodMetrics{
		{Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1, Containers: []k8stest.ContainerUsage{{Name: "nginx", CPU: "12m", Memory: "20Mi"}}},
		{Namespace: tuitest.NamespaceSystem, Name: tuitest.PodCoreDNS, Containers: []k8stest.ContainerUsage{{Name: "coredns", CPU: "3m", Memory: "15Mi"}}},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /apis/metrics.k8s.io/v1beta1/pods", k8stest.Forbidden())
	mux.Handle("/", k8stest.Metrics(pods, nil))
	return mux
}

func atLevel(msgs []tea.Msg, level status.Level) []string {
	var out []string
	for _, s := range tuitest.All[views.StatusMsg](msgs) {
		if s.Level == level {
			out = append(out, s.Text)
		}
	}
	return out
}

// TestForbiddenClusterWide is M4.3's fallback over the metrics API: with a
// context namespace top lands there with its rows, command and a hint;
// without one the error points at :ns.
func TestForbiddenClusterWide(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		contextNS   string
		level       status.Level
		want        string
		wantCommand string
		wantScreen  string
	}{
		{
			name: "context namespace", contextNS: tuitest.NamespaceDemo, level: status.Warn,
			want:        "cluster-wide list forbidden; showing demo · kubectl auth can-i list pods.metrics.k8s.io -A",
			wantCommand: "kubectl top pods -n demo --sort-by=cpu",
			wantScreen:  tuitest.PodWeb1,
		},
		{
			name: "no context namespace", level: status.Error,
			want:        "forbidden: list pods.metrics.k8s.io in all namespaces · :ns <name> picks one · kubectl auth can-i list pods.metrics.k8s.io -A",
			wantCommand: "kubectl top pods -A --sort-by=cpu",
			wantScreen:  "forbidden: list pods.metrics.k8s.io in all namespaces · :ns <name> picks one",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, metricsDeniedClusterWide())
			c.ContextNamespace = tc.contextNS
			m, _ := fresh(c, "")
			m, out := tuitest.Mount(t, m, width, height)
			if got := atLevel(out, tc.level); len(got) != 1 || got[0] != tc.want {
				t.Errorf("statuses %+v, want one %v %q", tuitest.All[views.StatusMsg](out), tc.level, tc.want)
			}
			if got := m.Command().String(); got != tc.wantCommand {
				t.Errorf("Command = %q, want %q", got, tc.wantCommand)
			}
			screen := tuitest.Screen(m)
			if !strings.Contains(strings.Join(strings.Fields(screen), " "), tc.wantScreen) {
				t.Errorf("screen lacks %q:\n%s", tc.wantScreen, screen)
			}
			if tc.contextNS != "" && (strings.Contains(screen, "NAMESPACE") || strings.Contains(screen, tuitest.PodCoreDNS)) {
				t.Errorf("the fallback still shows every namespace:\n%s", screen)
			}
		})
	}
}
