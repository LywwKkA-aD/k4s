package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pod is the presentation-friendly snapshot of a Kubernetes Pod.
type Pod struct {
	Name      string
	Namespace string
	Ready     string // "1/2" — ready / total containers
	Status    string // pod phase or container waiting reason
	Restarts  int32  // sum across all containers
	Age       time.Duration
	IP        string
	Node      string
}

// ListPods returns pods in the given namespace, or in all namespaces when
// namespace == "".
func (c *Client) ListPods(ctx context.Context, namespace string) ([]Pod, error) {
	list, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Pod, 0, len(list.Items))
	now := time.Now()
	for i := range list.Items {
		p := &list.Items[i]
		out = append(out, Pod{
			Name:      p.Name,
			Namespace: p.Namespace,
			Ready:     readyString(p),
			Status:    podStatus(p),
			Restarts:  totalRestarts(p),
			Age:       now.Sub(p.CreationTimestamp.Time),
			IP:        p.Status.PodIP,
			Node:      p.Spec.NodeName,
		})
	}
	return out, nil
}

func readyString(p *corev1.Pod) string {
	ready := 0
	total := len(p.Spec.Containers)
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

// podStatus mirrors a simplified subset of kubectl's logic: a container's
// waiting / terminated reason takes precedence over the overall pod phase,
// because "CrashLoopBackOff" is more useful than "Running" when something is
// broken.
func podStatus(p *corev1.Pod) string {
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}
	return string(p.Status.Phase)
}

func totalRestarts(p *corev1.Pod) int32 {
	var sum int32
	for _, cs := range p.Status.ContainerStatuses {
		sum += cs.RestartCount
	}
	return sum
}
