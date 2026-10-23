package dash

import (
	"slices"
	"testing"
)

// versionGets counts the /version requests in log.
func versionGets(log []seen) int {
	n := 0
	for _, s := range log {
		if s.typed && s.resource == "version" {
			n++
		}
	}
	return n
}

// TestGivenVersionIsNeverAsked: the root asks /version for the header and
// hands the answer over, so a context costs one request. While the answer
// is on its way the version row waits; once given, the version names the
// distro and the plan never asks, not even on r.
func TestGivenVersionIsNeverAsked(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := NewSnapshot(Scope{}).GiveVersion("", "")
	if due := dueSet(MakePlan(s.PlanInput(0, true, true, watching))); slices.Contains(due, Version) {
		t.Fatalf("r plans /version the caller asks: %v", due)
	}
	s = tickAt(t, a, s, 0)
	if n := versionGets(a.take()); n != 0 {
		t.Fatalf("/version asked %d times while the caller's answer was on its way", n)
	}
	if v := s.Cluster.Version; v.GitVersion != "" || s.Caps.K3s {
		t.Fatalf("version before it was given = %+v, k3s %v", v, s.Caps.K3s)
	}

	s = s.GiveVersion("v1.36.5+k3s1", "36")
	if v := s.Cluster.Version; v.GitVersion != "v1.36.5+k3s1" || v.Distro != "k3s" || v.Avail.State != StateOK || !s.Caps.K3s {
		t.Fatalf("given version = %+v, k3s %v", v, s.Caps.K3s)
	}
	s = tickAt(t, a, s, 5*sec)
	if n := versionGets(a.take()); n != 0 || s.Cluster.Version.GitVersion != "v1.36.5+k3s1" {
		t.Fatalf("after a tick: %d /version asks, version %q", n, s.Cluster.Version.GitVersion)
	}
}

func TestGivenVersionOfAnOldServerFallsBackForLoadBalancers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, git, minor string
		fallback         bool
	}{
		{name: "before 1.31", git: "v1.30.4-eks-a737599", minor: "30+", fallback: true},
		{name: "1.31 and later", git: "v1.36.5+k3s1", minor: "36"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewSnapshot(Scope{}).GiveVersion(tc.git, tc.minor)
			if s.Caps.LBFallback != tc.fallback {
				t.Fatalf("LBFallback = %v, want %v", s.Caps.LBFallback, tc.fallback)
			}
		})
	}
}

// TestUngivenVersionIsAskedOnce: without a caller's version the dashboard
// asks /version itself, once per context.
func TestUngivenVersionIsAskedOnce(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := tickAt(t, a, NewSnapshot(Scope{}), 0)
	s = tickAt(t, a, s, 5*sec)
	if n := versionGets(a.take()); n != 1 || s.Cluster.Version.GitVersion == "" {
		t.Fatalf("/version asked %d times, version %q", n, s.Cluster.Version.GitVersion)
	}
}
