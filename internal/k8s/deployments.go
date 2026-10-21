package k8s

import (
	"context"
	"fmt"
	"time"
)

// Deployment is the presentation-friendly snapshot of an apps/v1 Deployment.
type Deployment struct {
	Name      string
	Namespace string
	Replicas  int32 // desired
	Ready     int32
	UpToDate  int32
	Available int32
	Age       time.Duration
}

// ListDeployments returns deployments in the given namespace, or in all
// namespaces when namespace == "", a consistent read unless opts say
// otherwise.
func (c *Client) ListDeployments(ctx context.Context, namespace string, opts ...ListOption) ([]Deployment, error) {
	list, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, listOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	out := make([]Deployment, 0, len(list.Items))
	now := time.Now()
	for i := range list.Items {
		d := &list.Items[i]
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		out = append(out, Deployment{
			Name:      d.Name,
			Namespace: d.Namespace,
			Replicas:  desired,
			Ready:     d.Status.ReadyReplicas,
			UpToDate:  d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			Age:       now.Sub(d.CreationTimestamp.Time),
		})
	}
	return out, nil
}
