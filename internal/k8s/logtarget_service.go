package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ServiceLogTarget resolves the pods behind a service's logs: every pod the
// selector matches except finished ones (Succeeded, Failed, Evicted),
// which are reported in Skipped. EndpointSlices are not used: they drop
// pods that are not Ready, the failing backends whose logs matter most.
// Containers come from the first streamed pod by name.
func (c *Client) ServiceLogTarget(ctx context.Context, namespace, name string) (LogTarget, error) {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return LogTarget{}, fmt.Errorf("get service: %w", err)
	}
	if len(svc.Spec.Selector) == 0 {
		return LogTarget{}, fmt.Errorf("service %s: %w", name, ErrNoSelector)
	}
	selector := labels.SelectorFromSet(svc.Spec.Selector).String()
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, fullList(selector))
	if err != nil {
		return LogTarget{}, fmt.Errorf("list pods: %w", err)
	}
	t := LogTarget{Kind: LogKindService, Namespace: namespace, Name: name, Selector: selector}
	for _, p := range byName(pods.Items) {
		if finished(p) {
			t.Skipped = append(t.Skipped, skippedPod(p, SkipFinished, true))
			continue
		}
		t.Pods = append(t.Pods, PodRef{Name: p.Name, Phase: string(p.Status.Phase), Hash: p.Labels[hashLabel]})
		if t.Containers == nil {
			t.Containers = containerNames(&p.Spec)
		}
	}
	return t, nil
}
