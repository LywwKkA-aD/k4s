package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Namespace is a presentation-friendly snapshot of a k8s Namespace.
type Namespace struct {
	Name   string
	Status corev1.NamespacePhase
	Age    metav1.Time
}

// ListNamespaces is the connection smoke test: a successful call proves the
// clientset is healthy. Future TUI views will swap this for a watcher.
func (c *Client) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Namespace, 0, len(list.Items))
	for _, ns := range list.Items {
		out = append(out, Namespace{
			Name:   ns.Name,
			Status: ns.Status.Phase,
			Age:    ns.CreationTimestamp,
		})
	}
	return out, nil
}
