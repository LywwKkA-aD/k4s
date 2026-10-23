package dashboard_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestCutCountsReadAsAtLeast: a count taken from a list the server cut
// (Limit and continue) reads N+, as every count in k4s does when it is a
// lower bound (M4.3): the services of panel 1 and the warnings of 5.
func TestCutCountsReadAsAtLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		change    func(*dash.Snapshot)
		want, not []string
	}{
		{
			name:   "services counted from a cut list",
			change: func(s *dash.Snapshot) { s.Cluster.Network.Services, s.Cluster.Network.ServicesApprox = 1, true },
			want:   []string{"○ network    1+ svc · 1 LoadBalancer <pending>"},
			not:    []string{"1 svc"},
		},
		{
			name:   "an exact service count",
			change: func(s *dash.Snapshot) { s.Cluster.Network.Services = 102 },
			want:   []string{"102 svc"},
			not:    []string{"102+ svc"},
		},
		{
			name:   "a capped page of warnings",
			change: func(s *dash.Snapshot) { s.Events.Total, s.Events.Truncated = 2000, true },
			want:   []string{"2000+ warnings", "newest first · 10 of 2000+"},
			not:    []string{"≥"},
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
			requireAll(t, tuitest.Screen(mount(t, newFeed(base), wideW, wideH)), tt.want, tt.not)
		})
	}
}
