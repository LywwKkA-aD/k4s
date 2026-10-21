package services_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// denyAllNamespaces makes RBAC refuse a service list across every
// namespace; lists in one namespace go through.
func denyAllNamespaces() tuitest.FakeOption {
	return tuitest.WithReactor("list", "services", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() != "" {
			return false, nil, nil
		}
		denial := errors.New(`User "u" cannot list resource "services" in API group "" at the cluster scope`)
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "", denial)
	})
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

// TestForbiddenClusterWide is M4.3's fallback: with a context namespace the
// view lands there with its rows, command and a hint; without one the
// error points at :ns.
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
			want:        "cluster-wide list forbidden; showing demo · kubectl auth can-i list services -A",
			wantCommand: "kubectl get services -n demo",
			wantScreen:  "ClusterIP",
		},
		{
			name: "no context namespace", level: status.Error,
			want:        "forbidden: list services in all namespaces · :ns <name> picks one · kubectl auth can-i list services -A",
			wantCommand: "kubectl get services -A",
			wantScreen:  "services unavailable: forbidden: list services in all namespaces",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, denyAllNamespaces())
			c.ContextNamespace = tc.contextNS
			m, _ := clocked(c, "")
			m, out := tuitest.Mount(t, m, width, height)
			if got := atLevel(out, tc.level); len(got) != 1 || got[0] != tc.want {
				t.Errorf("statuses %+v, want one %v %q", tuitest.All[views.StatusMsg](out), tc.level, tc.want)
			}
			if got := m.Command().String(); got != tc.wantCommand {
				t.Errorf("Command = %q, want %q", got, tc.wantCommand)
			}
			screen := tuitest.Screen(m)
			if !strings.Contains(screen, tc.wantScreen) {
				t.Errorf("screen lacks %q:\n%s", tc.wantScreen, screen)
			}
			if tc.contextNS != "" && (strings.Contains(screen, "NAMESPACE") || !strings.Contains(screen, tuitest.ServiceWeb)) {
				t.Errorf("want the demo services without a NAMESPACE column:\n%s", screen)
			}
		})
	}
}
