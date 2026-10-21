package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReresolveLogTarget resolves t again while its logs are open: the pods
// its selector matches now, with their state, so the logs view can follow
// new pods and explain the streams that end. It makes one call, a List of
// pods by t's own selector from the watch cache. A deployment's
// ReplicaSets are listed again, as a quorum read, only when a pod is
// controlled by one named like the deployment's that t has never seen: a
// rollout. Owners guessed stay guessed, and t's Containers are kept.
//
// A deployment target that does not carry its UID (one not built by
// DeploymentLogTarget) is resolved afresh when it meets an unknown
// ReplicaSet.
func (c *Client) ReresolveLogTarget(ctx context.Context, t LogTarget) (LogTarget, error) {
	if t.Kind != LogKindDeployment && t.Kind != LogKindService {
		return LogTarget{}, fmt.Errorf("%w: %q", ErrUnsupportedKind, t.Kind)
	}
	if t.Selector == "" {
		return LogTarget{}, fmt.Errorf("%s %s: %w", t.Kind, t.Name, ErrNoSelector)
	}
	pods, err := c.Clientset.CoreV1().Pods(t.Namespace).List(ctx, fullList(t.Selector))
	if err != nil {
		return LogTarget{}, fmt.Errorf("list pods: %w", err)
	}
	next := LogTarget{
		Kind:       t.Kind,
		Namespace:  t.Namespace,
		Name:       t.Name,
		Selector:   t.Selector,
		Containers: t.Containers,
		UID:        t.UID,
	}
	if t.Kind == LogKindService {
		return servicePods(next, pods.Items), nil
	}
	owners := knownReplicaSets(t)
	if owners.missesOwnerOf(pods.Items) {
		if t.UID == "" {
			return c.DeploymentLogTarget(ctx, t.Namespace, t.Name)
		}
		fresh := metav1.ListOptions{LabelSelector: t.Selector}
		again, err := c.replicaSetsOf(ctx, t.Namespace, t.Name, t.UID, fresh)
		if err != nil {
			return LogTarget{}, err
		}
		owners = again
	}
	return owners.split(next, byName(pods.Items)), nil
}
