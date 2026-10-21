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

// ListOption adjusts a list request.
type ListOption func(metav1.ListOptions) metav1.ListOptions

// FromCache lets the apiserver answer a list from its watch cache
// (ResourceVersion "0") instead of a consistent read. The answer may lag
// by a moment, which a refresh tick does not mind; on k3s it spares kine
// a full read on every tick. Without it a list is a consistent read, as
// a manual refresh wants.
func FromCache() ListOption {
	return func(o metav1.ListOptions) metav1.ListOptions {
		o.ResourceVersion = "0"
		return o
	}
}

func listOptions(opts []ListOption) metav1.ListOptions {
	var o metav1.ListOptions
	for _, opt := range opts {
		o = opt(o)
	}
	return o
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
