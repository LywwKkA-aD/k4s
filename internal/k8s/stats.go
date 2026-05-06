package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Stats is a coarse-grained snapshot of cluster contents, suitable for the
// landing dashboard. It is not authoritative — for the per-resource views we
// will eventually use informer-backed watchers.
type Stats struct {
	Namespaces  int
	Pods        int
	Deployments int
	Services    int
}

// Stats issues four list calls (namespaces, pods, deployments, services) and
// returns counts. List calls are sequential — for four cheap calls the extra
// goroutine plumbing is not worth it.
func (c *Client) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	opts := metav1.ListOptions{}

	ns, err := c.Clientset.CoreV1().Namespaces().List(ctx, opts)
	if err != nil {
		return s, err
	}
	s.Namespaces = len(ns.Items)

	pods, err := c.Clientset.CoreV1().Pods("").List(ctx, opts)
	if err != nil {
		return s, err
	}
	s.Pods = len(pods.Items)

	deps, err := c.Clientset.AppsV1().Deployments("").List(ctx, opts)
	if err != nil {
		return s, err
	}
	s.Deployments = len(deps.Items)

	svcs, err := c.Clientset.CoreV1().Services("").List(ctx, opts)
	if err != nil {
		return s, err
	}
	s.Services = len(svcs.Items)

	return s, nil
}
