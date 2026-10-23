package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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
	t := servicePods(LogTarget{Kind: LogKindService, Namespace: namespace, Name: name, Selector: selector}, pods.Items)
	if len(t.Pods) > 0 {
		t.Containers, t.Init = t.Pods[0].Containers, t.Pods[0].Init
	}
	return t, nil
}

// servicePods fills t with the pods the selector matched: finished ones
// skipped, the rest streamed, both by name.
func servicePods(t LogTarget, items []corev1.Pod) LogTarget {
	for _, p := range byName(items) {
		if finished(p) {
			t.Skipped = append(t.Skipped, skippedPod(p, SkipFinished, true))
			continue
		}
		t.Pods = append(t.Pods, podRef(p))
	}
	return t
}
