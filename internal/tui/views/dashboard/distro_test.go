package dashboard_test

import (
	"strings"
	"testing"

	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// TestMetricsHintFollowsTheDistro: panel 2 words a missing metrics-server
// with cluster.MetricsHint, for the distribution the root detected
// (WithDistro), else the one the data layer read from the version.
func TestMetricsHintFollowsTheDistro(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []dashboard.Option
		want string
	}{
		{name: "minikube from the root", opts: []dashboard.Option{dashboard.WithDistro(cluster.Minikube)}, want: cluster.MetricsHint(cluster.Minikube)},
		{name: "k3d runs k3s", opts: []dashboard.Option{dashboard.WithDistro(cluster.K3d)}, want: cluster.MetricsHint(cluster.K3s)},
		{name: "nothing detected yet: the version's", opts: []dashboard.Option{dashboard.WithDistro(cluster.Other)}, want: cluster.MetricsHint(cluster.Other)},
		{name: "no root: the version's", want: cluster.MetricsHint(cluster.Other)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFeed(vanillaKind)
			m := dashboard.New(tuitest.NewFakeClient(t), nil, append(f.options(), tt.opts...)...)
			m, _ = tuitest.Mount(t, m, 300, wideH)
			if got := tuitest.Screen(m); !strings.Contains(got, tt.want) {
				t.Fatalf("panel 2 lacks %q:\n%s", tt.want, got)
			}
		})
	}
}

// TestTheRootsDistroArrivesWithItsVersion: a dashboard built before the
// root knew the distribution learns it with the version.
func TestTheRootsDistroArrivesWithItsVersion(t *testing.T) {
	t.Parallel()

	f := newFeed(vanillaKind)
	opts := append(f.options(), dashboard.WithDistro(cluster.Other), dashboard.WithServerVersion(nil))
	m, _ := tuitest.Mount(t, dashboard.New(tuitest.NewFakeClient(t), nil, opts...), 300, wideH)
	m, _ = tuitest.Drive(t, m, dashboard.VersionMsg{Info: &apiversion.Info{GitVersion: "v1.33.1"}, Distro: cluster.Minikube})
	if got := tuitest.Screen(m); !strings.Contains(got, cluster.MetricsHint(cluster.Minikube)) {
		t.Fatalf("panel 2 kept the old hint:\n%s", got)
	}
}
