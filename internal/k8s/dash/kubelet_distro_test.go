package dash

import (
	"errors"
	"slices"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// TestKubeletsNameTheDistributionWithoutAVersion: while /version is denied
// or not answered, the nodes list the dashboard fetches anyway names the
// distribution through the kubelets' versions (v1.36.3+k3s1), so a k3s
// cluster still shows its k3s panel and plans its k3s lists. A server
// version, once known, is the one that counts.
func TestKubeletsNameTheDistributionWithoutAVersion(t *testing.T) {
	t.Parallel()

	forbidden := apierrors.NewForbidden(versionResource, "", errors.New("rbac"))
	cases := []struct {
		name    string
		snap    func(Snapshot) Snapshot
		k3s     bool
		distro  string
		k3sPlan bool
	}{
		{name: "version denied", snap: func(s Snapshot) Snapshot { return s.GiveVersionError(forbidden) }, k3s: true, distro: "k3s", k3sPlan: true},
		{name: "version on its way", snap: func(s Snapshot) Snapshot { return s }, k3s: true, distro: "k3s", k3sPlan: true},
		{name: "a server version without a distribution", snap: func(s Snapshot) Snapshot { return s.GiveVersion("v1.36.3", "36") }},
		{name: "a server version naming another", snap: func(s Snapshot) Snapshot { return s.GiveVersion("v1.36.3+rke2r1", "36") }, distro: "rke2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newAPIServer(t, busyK3s(fixedNow))
			s := tc.snap(tickAt(t, a, NewSnapshot(Scope{}).GiveVersion("", ""), 0))
			if s.Caps.K3s != tc.k3s || s.Cluster.Version.Distro != tc.distro || s.K3s.Shown != tc.k3s {
				t.Fatalf("k3s %v, distro %q, panel shown %v; want %v, %q", s.Caps.K3s, s.Cluster.Version.Distro, s.K3s.Shown, tc.k3s, tc.distro)
			}
			due := dueSet(MakePlan(s.PlanInput(0, true, true, watching)))
			if slices.Contains(due, HelmCharts) != tc.k3sPlan {
				t.Fatalf("helm charts planned = %v, want %v: %v", !tc.k3sPlan, tc.k3sPlan, due)
			}
		})
	}
}

func TestDistroOfTheKubelets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kubelets []string
		want     string
	}{
		{name: "no nodes"},
		{name: "k3s", kubelets: []string{"v1.36.3+k3s1", "v1.36.3+k3s1"}, want: "k3s"},
		{name: "k3s agents a release behind", kubelets: []string{"v1.36.3+k3s1", "v1.35.9+k3s1"}, want: "k3s"},
		{name: "eks", kubelets: []string{"v1.30.4-eks-a737599"}, want: "eks"},
		{name: "vanilla", kubelets: []string{"v1.36.3"}},
		{name: "kubelets that disagree", kubelets: []string{"v1.36.3+k3s1", "v1.36.3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var st store
			for _, k := range tc.kubelets {
				st.nodes = append(st.nodes, nodeInfo{kubelet: k})
			}
			if got := st.distro(); got != tc.want {
				t.Fatalf("distro = %q, want %q", got, tc.want)
			}
		})
	}
}
