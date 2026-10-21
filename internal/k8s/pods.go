package k8s

import (
	"context"
	"time"
)

// Pod is the presentation-friendly snapshot of a Kubernetes Pod. Ready,
// Status and Restarts follow `kubectl get pods` (see PodColumnsFor).
type Pod struct {
	Name      string
	Namespace string
	Ready     string // "1/2": ready / total, native sidecars included
	Status    string // kubectl's STATUS: Running, Init:0/1, Evicted, Terminating...
	Restarts  int32  // kubectl's RESTARTS count
	// RestartsText is kubectl's RESTARTS cell, "3 (5m ago)", as of the list.
	RestartsText string
	Age          time.Duration
	IP           string
	Node         string
}

// ListPods returns pods in the given namespace, or in all namespaces when
// namespace == "", a consistent read unless opts say otherwise.
func (c *Client) ListPods(ctx context.Context, namespace string, opts ...ListOption) ([]Pod, error) {
	list, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, listOptions(opts))
	if err != nil {
		return nil, listError("pods", namespace, err)
	}
	out := make([]Pod, 0, len(list.Items))
	now := time.Now()
	for i := range list.Items {
		p := &list.Items[i]
		cols := PodColumnsFor(p)
		out = append(out, Pod{
			Name:         p.Name,
			Namespace:    p.Namespace,
			Ready:        cols.Ready,
			Status:       cols.Status,
			Restarts:     cols.Restarts,
			RestartsText: cols.RestartsText(now),
			Age:          now.Sub(p.CreationTimestamp.Time),
			IP:           p.Status.PodIP,
			Node:         p.Spec.NodeName,
		})
	}
	return out, nil
}

// ContainersForPod returns the names of the pod's containers in picker
// order: the app containers, then the init containers and native
// sidecars, whose logs are where a pod stuck in Init: says why. The TUI
// prompts for a container when there is more than one.
func (c *Client) ContainersForPod(ctx context.Context, namespace, name string) ([]string, error) {
	cs, err := c.PodContainers(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	return cs.Names(), nil
}
