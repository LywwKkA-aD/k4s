package events_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// restricted is a client whose only grant is a Role in demo, with
// contextNS as the namespace its kubeconfig context sets.
func restricted(contextNS string) *k8s.Client {
	c, cs := k8stest.NewFakeClient(fixture()...)
	k8stest.RestrictToNamespace(cs, demo)
	c.ContextNamespace = contextNS
	return c
}

// TestForbiddenClusterWideFallsBackToTheContextNamespace (M4.3): -A is
// forbidden and the context names a namespace, so the view lists that one
// and says why.
func TestForbiddenClusterWideFallsBackToTheContextNamespace(t *testing.T) {
	t.Parallel()

	m, out := mounted(t, restricted(demo), "")
	requireStatus(t, out, status.Warn, "cluster-wide list forbidden; showing demo · kubectl auth can-i list events -A")
	got := rows(m)
	if len(got) != 3 || !strings.Contains(got[0], "BackOff") {
		t.Errorf("rows after the fallback:\n%s", strings.Join(got, "\n"))
	}
	if strings.Contains(strings.Join(screenLines(m), "\n"), "NAMESPACE") {
		t.Error("a one-namespace list shows the NAMESPACE column")
	}
	if !strings.Contains(tuitest.Screen(m), "namespace demo: cluster-wide list forbidden") {
		t.Errorf("no note above the rows:\n%s", tuitest.Screen(m))
	}
	if c := m.Command().String(); c != "kubectl events -n demo" {
		t.Errorf("Command = %q", c)
	}
}

func TestForbiddenWithoutAFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		contextNS string
		namespace string
		want      string
	}{
		{
			name: "cluster-wide, no context namespace",
			want: "forbidden: list events in all namespaces · :ns <name> picks one · kubectl auth can-i list events -A",
		},
		{
			name: "another namespace", contextNS: demo, namespace: "other",
			want: "forbidden: list events in other · kubectl auth can-i list events -n other",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := mounted(t, restricted(tc.contextNS), tc.namespace)
			requireStatus(t, out, status.Error, tc.want)
			screen := tuitest.Screen(m)
			if !strings.Contains(screen, "events unavailable: "+tc.want) || !strings.Contains(screen, "is forbidden") {
				t.Errorf("body:\n%s", screen)
			}
		})
	}
}
