package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Namespace is a presentation-friendly snapshot of a k8s Namespace.
type Namespace struct {
	Name   string
	Status corev1.NamespacePhase
	Age    metav1.Time
}

// ListNamespaces lists the cluster's namespaces, a consistent read unless
// opts say otherwise.
func (c *Client) ListNamespaces(ctx context.Context, opts ...ListOption) ([]Namespace, error) {
	list, err := c.Clientset.CoreV1().Namespaces().List(ctx, listOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
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
