package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// podStatus is the STATUS the pods table shows; describe reuses it so the
// two screens agree.
func podStatus(p *corev1.Pod) string {
	return PodColumnsFor(p).Status
}

// ContainersForPod returns the names of every container in the pod's spec.
// Used by the TUI to decide whether to prompt for a container before opening
// logs or exec.
func (c *Client) ContainersForPod(ctx context.Context, namespace, name string) ([]string, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}
	out := make([]string, 0, len(pod.Spec.Containers))
	for _, ct := range pod.Spec.Containers {
		out = append(out, ct.Name)
	}
	return out, nil
}
