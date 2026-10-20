package forwards

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// tunneler is the part of *k8s.Client the Manager drives. Declaring it here
// lets tests hand the Manager a fake whose sessions they control.
type tunneler interface {
	StartPodPortForward(ctx context.Context, namespace, podName string, localPort, remotePort uint16) (*k8s.PortForwardSession, error)
	ResolveServiceToPod(ctx context.Context, namespace, serviceName string, svcPort int32) (*corev1.Pod, int32, error)
	ResolveDeploymentToPod(ctx context.Context, namespace, deployName string) (string, error)
}

var _ tunneler = (*k8s.Client)(nil)

// cluster names the cluster a Manager serves. It is read once from the
// client instead of through tunneler methods because *k8s.Client already
// has an exported Context field, which rules out a Context() method.
type cluster struct {
	contextName string
	server      string
}

func clusterOf(c *k8s.Client) cluster {
	if c == nil {
		return cluster{}
	}
	id := cluster{contextName: c.Context}
	if c.RestConfig != nil {
		id.server = c.RestConfig.Host
	}
	return id
}

// deps are the Manager's collaborators. NewManager fills them from a
// *k8s.Client; tests swap in fakes and a temporary state file.
type deps struct {
	tunnel    tunneler // nil when k4s runs without a kubeconfig
	cluster   cluster
	probePort func(port uint16) error
	statePath string
}
