package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// switched is a session after :ctx prod: a pasted command needs --context
// to reach the cluster k4s shows, not the kubeconfig's current one.
var switched = kubectl.Session{Context: "prod", CurrentContext: "dev"}

func mountIn(t *testing.T, s kubectl.Session, f *feed) dashboard.Model {
	t.Helper()
	m := dashboard.New(tuitest.NewFakeClient(t), nil, append(f.options(), dashboard.WithSession(s))...)
	m, _ = tuitest.Mount(t, m, wideW, wideH)
	return m
}

// TestCommandsInTheBodyCarryTheSession: every command the body prints for
// pasting (the next suggestion, the command of the day, a denied source's
// probe) and the help map's commands reach the same cluster as the footer.
// The footer command itself stays bare: the root decorates it.
func TestCommandsInTheBodyCarryTheSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base func() dash.Snapshot
		want string
	}{
		{
			name: "next suggestion",
			base: busyK3s,
			want: "kubectl --context prod logs payments-worker-6c9f7d8b5-xk2lp -n shop -c worker --previous",
		},
		{
			name: "command of the day",
			base: allClear,
			want: "kubectl --context prod get pods -A",
		},
		{
			name: "a denied source's probe",
			base: func() dash.Snapshot {
				s := busyK3s()
				s.Pods = dash.PodsPanel{Avail: denied("auth", "can-i", "list", "pods", "-A"), Cmd: s.Pods.Cmd}
				return s
			},
			want: "kubectl --context prod auth can-i list pods -A",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := mountIn(t, switched, newFeed(tt.base))
			if screen := tuitest.Screen(m); !strings.Contains(screen, tt.want) {
				t.Fatalf("screen lacks %q:\n%s", tt.want, screen)
			}
			if got := command(m); strings.Contains(got, "--context") {
				t.Fatalf("footer command = %q, want it left to the root to decorate", got)
			}
		})
	}
}

func TestHelpMapCarriesTheSession(t *testing.T) {
	t.Parallel()

	m := mountIn(t, switched, newFeed(busyK3s))
	if desc := m.HelpMap()[3].Help().Desc; desc != "pods · kubectl --context prod get pods -A" {
		t.Fatalf("map entry 3 = %q", desc)
	}
}

// TestNoSessionLeavesCommandsBare: a session that needs no flags (k4s on
// the kubeconfig's current context) prints the commands as they are.
func TestNoSessionLeavesCommandsBare(t *testing.T) {
	t.Parallel()

	m := mountIn(t, kubectl.Session{Context: "dev", CurrentContext: "dev"}, newFeed(busyK3s))
	if screen := tuitest.Screen(m); strings.Contains(screen, "--context") {
		t.Fatalf("screen carries --context without a switch:\n%s", screen)
	}
}
