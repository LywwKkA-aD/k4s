package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// namespaces when namespace == "".
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]Deployment, error) {
	list, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
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

// PodsForDeployment returns the names of pods that match the deployment's
// label selector. Used by the TUI to tail every replica of a deployment in
// one shot via the logs view's multi-pod streaming.
func (c *Client) PodsForDeployment(ctx context.Context, namespace, name string) ([]string, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	if dep.Spec.Selector == nil {
		return nil, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("convert selector: %w", err)
	}
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	names := make([]string, 0, len(pods.Items))
	for _, p := range pods.Items {
		names = append(names, p.Name)
	}
	return names, nil
}
