// Package cluster names the Kubernetes distribution and the cluster k4s is
// connected to, and words hints for the distribution. It works only from
// data k4s already has (the server version, the kubeconfig) and makes no
// API calls itself.
package cluster

import "strings"

// Distro is a Kubernetes distribution.
type Distro string

// Distributions Detect can tell apart.
const (
	K3s      Distro = "k3s"
	RKE2     Distro = "rke2"
	EKS      Distro = "eks"
	GKE      Distro = "gke"
	AKS      Distro = "aks"
	K3d      Distro = "k3d"
	Kind     Distro = "kind"
	Minikube Distro = "minikube"
	Other    Distro = "other"
)

// IsK3s reports whether d runs k3s, so k3s-specific features apply. k3d
// runs k3s in containers.
func (d Distro) IsK3s() bool {
	return d == K3s || d == K3d
}

// Detect names the distribution from the server's /version gitVersion ("" if
// unknown), the kubeconfig context name and the nodes' spec.providerID
// values (nil when nodes cannot be listed). The gitVersion suffix wins, then
// the context names that k3d, kind and minikube generate, then provider ids.
func Detect(gitVersion, contextName string, providerIDs []string) Distro {
	if d, ok := fromVersion(gitVersion); ok {
		if d == K3s && isK3dContext(contextName) {
			return K3d
		}
		return d
	}
	if d, ok := fromContext(contextName); ok {
		return d
	}
	if d, ok := fromProviders(providerIDs, gitVersion == ""); ok {
		if d == K3s && isK3dContext(contextName) {
			return K3d
		}
		return d
	}
	return Other
}

// versionMarkers are the build-metadata suffixes vendors put in gitVersion,
// e.g. v1.36.5+k3s1, v1.30.4-eks-a737599, v1.30.5-gke.1014001.
var versionMarkers = []struct {
	marker string
	distro Distro
}{
	{"+k3s", K3s},
	{"+rke2", RKE2},
	{"-eks-", EKS},
	{"-gke.", GKE},
}

func fromVersion(gitVersion string) (Distro, bool) {
	for _, m := range versionMarkers {
		if strings.Contains(gitVersion, m.marker) {
			return m.distro, true
		}
	}
	return "", false
}

func isK3dContext(name string) bool {
	return strings.HasPrefix(name, "k3d-")
}

func fromContext(name string) (Distro, bool) {
	switch {
	case isK3dContext(name):
		return K3d, true
	case strings.HasPrefix(name, "kind-"):
		return Kind, true
	case name == "minikube" || strings.HasPrefix(name, "minikube-"):
		return Minikube, true
	}
	return "", false
}

// providerSchemes maps spec.providerID schemes to distributions. aws and gce
// only count when the version is unknown: managed EKS and GKE builds carry a
// version suffix, so a plain build on those clouds is kubeadm, kops or alike.
var providerSchemes = []struct {
	scheme       string
	distro       Distro
	needsUnknown bool
}{
	{"k3s://", K3s, false},
	{"rke2://", RKE2, false},
	{"kind://", Kind, false},
	{"azure://", AKS, false},
	{"aws://", EKS, true},
	{"gce://", GKE, true},
}

func fromProviders(ids []string, versionUnknown bool) (Distro, bool) {
	for _, id := range ids {
		for _, s := range providerSchemes {
			if s.needsUnknown && !versionUnknown {
				continue
			}
			if strings.HasPrefix(id, s.scheme) {
				return s.distro, true
			}
		}
	}
	return "", false
}
