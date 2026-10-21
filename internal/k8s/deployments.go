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
// namespaces when namespace == "". It is a consistent read.
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]Deployment, error) {
	return c.ListDeploymentsAt(ctx, namespace, "")
}

// ListDeploymentsAt is ListDeployments at resourceVersion: "0" lets the
// apiserver answer from its watch cache, which is what a poll wants (on
// k3s it spares kine a query); "" is a consistent read, for an explicit
// refresh.
func (c *Client) ListDeploymentsAt(ctx context.Context, namespace, resourceVersion string) ([]Deployment, error) {
	list, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{ResourceVersion: resourceVersion})
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

// ContainersForDeployment returns the container names defined in the
// deployment's pod template. Every replica has the same set, so the TUI
// resolves it once and reuses it for the whole deployment.
//
// Superseded by DeploymentLogTarget, which returns the containers with the
// pods in one resolve. It stays until the deployments view moves over; the
// formal deprecation marker would fail staticcheck on that caller first.
func (c *Client) ContainersForDeployment(ctx context.Context, namespace, name string) ([]string, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	return containerNames(&dep.Spec.Template.Spec), nil
}

// PodsForDeployment returns the names of pods that match the deployment's
// label selector. Used by the TUI to tail every replica of a deployment in
// one shot via the logs view's multi-pod streaming.
//
// Superseded by DeploymentLogTarget: the selector alone also matches
// finished pods and pods of other owners that share the labels, such as a
// CronJob's. It stays until the deployments view moves over.
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
