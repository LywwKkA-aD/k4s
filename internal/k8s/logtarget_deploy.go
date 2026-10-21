package k8s

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	hashLabel          = appsv1.DefaultDeploymentUniqueLabelKey
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

// DeploymentLogTarget resolves the pods behind a deployment's logs by
// owner, not by label alone: the pods its ReplicaSets control, minus
// finished ones. Pods that only share the labels, such as a CronJob's,
// are skipped. It makes exactly three calls: get the deployment, then
// list its ReplicaSets and its pods by the deployment's own selector from
// the watch cache.
//
// When listing ReplicaSets is forbidden, ownership falls back to the
// ReplicaSet name the deployment controller gives (<deployment>-<hash>)
// and OwnersGuessed is set.
func (c *Client) DeploymentLogTarget(ctx context.Context, namespace, name string) (LogTarget, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return LogTarget{}, fmt.Errorf("get deployment: %w", err)
	}
	selector, err := deploymentSelector(dep)
	if err != nil {
		return LogTarget{}, err
	}
	owners, err := c.replicaSetsOf(ctx, dep, selector)
	if err != nil {
		return LogTarget{}, err
	}
	// The bare selector, never the hash one: pods of a ReplicaSet created
	// since the footer was built must still be found.
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, fullList(selector))
	if err != nil {
		return LogTarget{}, fmt.Errorf("list pods: %w", err)
	}
	t := LogTarget{
		Kind:          LogKindDeployment,
		Namespace:     namespace,
		Name:          name,
		Selector:      selector,
		Containers:    containerNames(&dep.Spec.Template.Spec),
		OwnersGuessed: owners.guessed,
	}
	return owners.split(t, byName(pods.Items)), nil
}

func deploymentSelector(dep *appsv1.Deployment) (string, error) {
	if dep.Spec.Selector == nil {
		return "", fmt.Errorf("deployment %s: %w", dep.Name, ErrNoSelector)
	}
	sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("convert selector: %w", err)
	}
	if sel.Empty() {
		return "", fmt.Errorf("deployment %s: %w", dep.Name, ErrNoSelector)
	}
	return sel.String(), nil
}

// replicaSets identifies a deployment's pods: by the revision of each
// ReplicaSet it controls, or, when guessed, by the controller's name rule.
type replicaSets struct {
	deployment string
	revision   map[types.UID]int64
	guessed    bool
}

func (c *Client) replicaSetsOf(ctx context.Context, dep *appsv1.Deployment, selector string) (replicaSets, error) {
	list, err := c.Clientset.AppsV1().ReplicaSets(dep.Namespace).List(ctx, fullList(selector))
	if apierrors.IsForbidden(err) {
		return replicaSets{deployment: dep.Name, guessed: true}, nil
	}
	if err != nil {
		return replicaSets{}, fmt.Errorf("list replicasets: %w", err)
	}
	revision := make(map[types.UID]int64, len(list.Items))
	for i := range list.Items {
		rs := &list.Items[i]
		if ref := metav1.GetControllerOfNoCopy(rs); ref != nil && ref.UID == dep.UID {
			revision[rs.UID] = revisionOf(rs)
		}
	}
	return replicaSets{deployment: dep.Name, revision: revision}, nil
}

// revisionOf reads the controller's revision stamp; a missing or broken
// one sorts as oldest.
func revisionOf(rs *appsv1.ReplicaSet) int64 {
	n, err := strconv.ParseInt(rs.Annotations[revisionAnnotation], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// owns reports whether p is the deployment's, with the rank its hash sorts
// by, newest first: the ReplicaSet revision, or when guessed the pod's age.
func (s replicaSets) owns(p *corev1.Pod) (bool, int64) {
	ref := metav1.GetControllerOfNoCopy(p)
	if ref == nil || ref.Kind != "ReplicaSet" {
		return false, 0
	}
	if !s.guessed {
		rev, ok := s.revision[ref.UID]
		return ok, rev
	}
	hash := p.Labels[hashLabel]
	return hash != "" && ref.Name == s.deployment+"-"+hash, p.CreationTimestamp.Unix()
}

// split sorts pods into streamed and skipped and derives the hashes the
// footer narrows the selector to.
func (s replicaSets) split(t LogTarget, pods []*corev1.Pod) LogTarget {
	rank := map[string]int64{}
	everyHash := true
	var finishedPods, otherPods []*corev1.Pod
	for _, p := range pods {
		owned, r := s.owns(p)
		switch {
		case !owned:
			otherPods = append(otherPods, p)
		case finished(p):
			finishedPods = append(finishedPods, p)
		default:
			hash := p.Labels[hashLabel]
			t.Pods = append(t.Pods, PodRef{Name: p.Name, Phase: string(p.Status.Phase), Hash: hash})
			if hash == "" {
				everyHash = false
				continue
			}
			if old, seen := rank[hash]; !seen || r > old {
				rank[hash] = r
			}
		}
	}
	if everyHash {
		t.Hashes = rankedHashes(rank)
	}
	shown := func(p *corev1.Pod) bool {
		return len(t.Hashes) == 0 || slices.Contains(t.Hashes, p.Labels[hashLabel])
	}
	for _, p := range finishedPods {
		t.Skipped = append(t.Skipped, skippedPod(p, SkipFinished, shown(p)))
	}
	for _, p := range otherPods {
		t.Skipped = append(t.Skipped, skippedPod(p, SkipOtherOwner, shown(p)))
	}
	slices.SortFunc(t.Skipped, func(a, b SkippedPod) int { return cmp.Compare(a.Name, b.Name) })
	return t
}

// rankedHashes returns the hashes by rank, highest first, ties by name;
// nil for none.
func rankedHashes(rank map[string]int64) []string {
	if len(rank) == 0 {
		return nil
	}
	out := make([]string, 0, len(rank))
	for h := range rank {
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b string) int {
		if c := cmp.Compare(rank[b], rank[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	return out
}

// byName returns pointers to items sorted by name, leaving items alone.
func byName(items []corev1.Pod) []*corev1.Pod {
	out := make([]*corev1.Pod, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	slices.SortFunc(out, func(a, b *corev1.Pod) int { return cmp.Compare(a.Name, b.Name) })
	return out
}
