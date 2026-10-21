package cluster

import "testing"

func TestDetect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		gitVersion  string
		context     string
		providerIDs []string
		want        Distro
	}{
		// The server's gitVersion is the strongest signal.
		{name: "k3s build", gitVersion: "v1.36.5+k3s1", context: "default", want: K3s},
		{name: "k3s build on a k3d context", gitVersion: "v1.31.4+k3s1", context: "k3d-dev", want: K3d},
		{name: "rke2 build", gitVersion: "v1.30.4+rke2r1", context: "default", want: RKE2},
		{name: "eks build", gitVersion: "v1.30.4-eks-a737599", context: "arn:aws:eks:eu-west-1:1:cluster/prod", want: EKS},
		{name: "gke build", gitVersion: "v1.30.5-gke.1014001", context: "gke_proj_europe-west1_prod", want: GKE},
		{name: "version beats the context name", gitVersion: "v1.30.4-eks-a737599", context: "kind-test", want: EKS},
		{name: "version beats provider ids", gitVersion: "v1.36.5+k3s1", context: "lab", providerIDs: []string{"aws:///eu-west-1a/i-1"}, want: K3s},

		// Local clusters are named by their tools.
		{name: "k3d context, version unknown", context: "k3d-dev", want: K3d},
		{name: "kind context", gitVersion: "v1.33.1", context: "kind-k4s", want: Kind},
		{name: "minikube context", gitVersion: "v1.33.1", context: "minikube", want: Minikube},
		{name: "minikube profile context", context: "minikube-arm", want: Minikube},
		{name: "a prefix alone is not a tool name", context: "kind", want: Other},

		// Provider ids fill in when the version says nothing.
		{name: "aks node", gitVersion: "v1.30.5", context: "prod", providerIDs: []string{"azure:///subscriptions/s/resourceGroups/g"}, want: AKS},
		{name: "kind node", context: "lab", providerIDs: []string{"kind://docker/k4s/k4s-control-plane"}, want: Kind},
		{name: "k3s node", context: "lab", providerIDs: []string{"k3s://node-1"}, want: K3s},
		{name: "k3s node on a k3d context", context: "k3d-x", providerIDs: []string{"k3s://k3d-x-server-0"}, want: K3d},
		{name: "rke2 node", context: "lab", providerIDs: []string{"rke2://node-1"}, want: RKE2},
		{name: "first known id wins", context: "lab", providerIDs: []string{"", "metal://1", "rke2://node-1", "k3s://node-2"}, want: RKE2},
		{name: "aws node, version unknown", context: "prod", providerIDs: []string{"aws:///eu-west-1a/i-1"}, want: EKS},
		{name: "gce node, version unknown", context: "prod", providerIDs: []string{"gce://proj/europe-west1-b/n1"}, want: GKE},
		// kubeadm or kops on cloud VMs: the version has no vendor suffix.
		{name: "aws node on a plain build", gitVersion: "v1.31.2", context: "prod", providerIDs: []string{"aws:///eu-west-1a/i-1"}, want: Other},
		{name: "gce node on a plain build", gitVersion: "v1.31.2", context: "prod", providerIDs: []string{"gce://proj/zone/n1"}, want: Other},

		{name: "nothing known", want: Other},
		{name: "plain build and context", gitVersion: "v1.31.2", context: "docker-desktop", want: Other},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Detect(tc.gitVersion, tc.context, tc.providerIDs); got != tc.want {
				t.Fatalf("Detect(%q, %q, %q) = %q, want %q", tc.gitVersion, tc.context, tc.providerIDs, got, tc.want)
			}
		})
	}
}

func TestDistroIsK3s(t *testing.T) {
	t.Parallel()

	cases := []struct {
		distro Distro
		want   bool
	}{
		{K3s, true},
		{K3d, true},
		{RKE2, false},
		{EKS, false},
		{Kind, false},
		{Other, false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(string(tc.distro), func(t *testing.T) {
			t.Parallel()
			if got := tc.distro.IsK3s(); got != tc.want {
				t.Fatalf("%q.IsK3s() = %v, want %v", tc.distro, got, tc.want)
			}
		})
	}
}
