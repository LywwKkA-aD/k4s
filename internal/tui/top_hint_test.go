package tui_test

import (
	"strings"
	"testing"

	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestTopHintFollowsTheDistribution: top on a cluster without a metrics
// API hints for the distribution the root knows when top opens: the one
// the preflight found, or the one /version named since the start.
func TestTopHintFollowsTheDistribution(t *testing.T) {
	t.Parallel()

	k3s := "metrics-server is bundled with k3s; disabled with --disable=metrics-server?"
	install := "install metrics-server: kubectl apply -f"
	cases := []struct {
		name       string
		context    string
		gitVersion string
		session    startup.Session
		want       string
		not        string
	}{
		{
			name: "named by /version", context: "prod", gitVersion: "v1.36.5+k3s1",
			session: startup.Session{Distro: cluster.Other}, want: k3s, not: install,
		},
		{
			// The preflight asked; the server is not asked again.
			name: "named by the preflight", context: "prod", gitVersion: "v1.30.4",
			session: startup.Session{Distro: cluster.K3s, ServerVersion: &apiversion.Info{GitVersion: "v1.36.5+k3s1"}},
			want:    k3s, not: install,
		},
		{
			name: "not k3s", context: "prod", gitVersion: "v1.30.4-eks-a737599",
			session: startup.Session{Distro: cluster.Other}, want: install, not: k3s,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := versionedClient(tc.context, tc.gitVersion)
			m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(tc.session), instantFrames()), screenW, screenH)
			m, _ = tuitest.Drive(t, m, colon("top")...)
			requireView(t, m, "top")
			screen := strings.Join(strings.Fields(tuitest.Screen(m)), " ")
			if !strings.Contains(screen, tc.want) {
				t.Errorf("top lacks %q:\n%s", tc.want, tuitest.Screen(m))
			}
			if strings.Contains(screen, tc.not) {
				t.Errorf("top shows %q:\n%s", tc.not, tuitest.Screen(m))
			}
		})
	}
}
