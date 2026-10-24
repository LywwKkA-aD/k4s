package k8s

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Kinds ResolveLogTarget accepts.
const (
	LogKindDeployment = "deployment"
	LogKindService    = "service"
)

var (
	// ErrNoSelector is returned for a workload that selects no pods by
	// label: a selector-less service, or a deployment without a selector.
	ErrNoSelector = errors.New("no selector")
	// ErrUnsupportedKind is returned by ResolveLogTarget for other kinds.
	ErrUnsupportedKind = errors.New("unsupported log target kind")
)

// SkipReason says why a pod the selector matches is not streamed.
type SkipReason string

// Reasons a matched pod is not streamed.
const (
	// SkipFinished marks a Succeeded or Failed pod (Evicted included):
	// its log is over, and `kubectl logs -f` on it only replays it.
	SkipFinished SkipReason = "finished"
	// SkipOtherOwner marks a pod that shares the labels but is not
	// controlled by the deployment's ReplicaSets, such as a CronJob's pod.
	SkipOtherOwner SkipReason = "other owner"
)

// PodRef is one pod a log target streams.
type PodRef struct {
	Name  string
	Phase string
	// Hash is the pod's pod-template-hash label; empty for none.
	Hash string
	// Created is when the pod was created: a pod that joins logs already
	// open is read from then, so nothing it wrote is lost.
	Created time.Time
	// Owner is the pod's controller as "Kind/name"; empty for none.
	Owner string
	// Containers are the pod's containers in picker order, the app
	// containers then the init containers, so a stream keeps an init
	// container the user picked and falls back to the pod's first app
	// container without asking for the pod.
	Containers []string
	// Init names the entries of Containers that are init containers or
	// native sidecars.
	Init []string
	// State is what a stream's end is explained by.
	State PodState
}

// SkippedPod is a pod the selector matches that k4s does not stream.
type SkippedPod struct {
	Name   string
	Phase  string
	Status string // the pod's reason (Evicted), a container's, or the phase
	Reason SkipReason
	// Owner is the pod's controller as "Kind/name"; empty for none.
	Owner string
	// InSelector is set when the shown selector matches the pod, so the
	// footer's `kubectl logs -l` would stream it.
	InSelector bool
	// State is the pod's state when it was listed.
	State PodState
}

// ReplicaSetRef is a ReplicaSet a deployment's log target has seen: one of
// the deployment's own with its revision, or another's that shares its
// labels. Re-resolving lists ReplicaSets again only for one it has never
// seen.
type ReplicaSetRef struct {
	UID      types.UID
	Revision int64
	Owned    bool
}

// LogTarget is the pod set behind the multi-pod logs of a deployment or a
// service, plus what the footer needs to show an exact kubectl command.
type LogTarget struct {
	Kind      string
	Namespace string
	Name      string
	// Selector is the workload's own selector in -l syntax.
	Selector string
	// Hashes are the pod-template-hash values of the deployment's pods
	// being streamed, newest ReplicaSet first. The shown selector is
	// Selector narrowed to them; none means Selector as is.
	Hashes []string
	// Pods are streamed, sorted by name.
	Pods []PodRef
	// Skipped are matched but not streamed, sorted by name.
	Skipped []SkippedPod
	// Containers are the deployment template's containers, or those of a
	// service's first streamed pod, in picker order: the app containers,
	// then the init containers and native sidecars (Containers.Names).
	Containers []string
	// Init names the entries of Containers that are init containers or
	// native sidecars (Containers.InitNames), which the picker labels.
	Init []string
	// Replicas is the deployment's spec.replicas, 1 when unset, so a
	// target without pods tells a deployment scaled to zero from one
	// whose pods are not up; 0 for a service.
	Replicas int32
	// OwnersGuessed is set when listing ReplicaSets was forbidden: a pod
	// counted as the deployment's when its controller is a ReplicaSet
	// named <deployment>-<pod-template-hash>.
	OwnersGuessed bool
	// UID is the deployment's, which tells its ReplicaSets from others;
	// empty for a service.
	UID types.UID
	// ReplicaSets are those seen so far, sorted by UID; nil for a service
	// or when the owners were guessed.
	ReplicaSets []ReplicaSetRef
}

// ResolveLogTarget resolves the pods to stream for a deployment or a
// service named name in namespace.
func (c *Client) ResolveLogTarget(ctx context.Context, kind, namespace, name string) (LogTarget, error) {
	switch kind {
	case LogKindDeployment:
		return c.DeploymentLogTarget(ctx, namespace, name)
	case LogKindService:
		return c.ServiceLogTarget(ctx, namespace, name)
	default:
		return LogTarget{}, fmt.Errorf("%w: %q", ErrUnsupportedKind, kind)
	}
}

// PodNames returns the names of the pods to stream.
func (t LogTarget) PodNames() []string {
	out := make([]string, 0, len(t.Pods))
	for _, p := range t.Pods {
		out = append(out, p.Name)
	}
	return out
}

// Matched counts the pods the shown selector matches: what
// --max-log-requests must cover for `kubectl logs -f -l` to start.
func (t LogTarget) Matched() int {
	n := len(t.Pods)
	for _, s := range t.Skipped {
		if s.InSelector {
			n++
		}
	}
	return n
}

// Approx reports whether the shown command differs from what k4s streams:
// it would stream skipped pods too, or the owners were guessed.
func (t LogTarget) Approx() bool {
	return t.OwnersGuessed || t.Matched() > len(t.Pods)
}

// SkipNote explains the skipped pods the shown command would still stream,
// such as "13 completed pods skipped (kubectl logs -l would include them)".
// Empty when there are none.
func (t LogTarget) SkipNote() string {
	kinds := []string{"completed", "failed", "evicted", "other"}
	counts := map[string]int{}
	total := 0
	for _, s := range t.Skipped {
		if s.InSelector {
			counts[skipKind(s)]++
			total++
		}
	}
	if total == 0 {
		return ""
	}
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if counts[k] > 0 {
			parts = append(parts, strconv.Itoa(counts[k])+" "+k)
		}
	}
	noun, pronoun := "pods", "them"
	if total == 1 {
		noun, pronoun = "pod", "it"
	}
	return fmt.Sprintf("%s %s skipped (kubectl logs -l would include %s)", joinAnd(parts), noun, pronoun)
}

func skipKind(s SkippedPod) string {
	switch {
	case s.Reason != SkipFinished:
		return "other"
	case s.Phase == string(corev1.PodSucceeded):
		return "completed"
	case s.Status == "Evicted":
		return "evicted"
	default:
		return "failed"
	}
}

// joinAnd joins parts as "a, b and c".
func joinAnd(parts []string) string {
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// fullList is a pod or ReplicaSet list served from the apiserver's watch
// cache: a log target needs the whole set, not a quorum read.
func fullList(selector string) metav1.ListOptions {
	return metav1.ListOptions{LabelSelector: selector, ResourceVersion: "0"}
}

func finished(p *corev1.Pod) bool {
	return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed
}

func skippedPod(p *corev1.Pod, reason SkipReason, inSelector bool) SkippedPod {
	return SkippedPod{
		Name:       p.Name,
		Phase:      string(p.Status.Phase),
		Status:     statusReason(p),
		Reason:     reason,
		Owner:      controllerName(p),
		InSelector: inSelector,
		State:      PodStateOf(p),
	}
}

func podRef(p *corev1.Pod) PodRef {
	cs := containersOf(p.Spec)
	return PodRef{
		Name:       p.Name,
		Phase:      string(p.Status.Phase),
		Hash:       p.Labels[hashLabel],
		Created:    p.CreationTimestamp.Time,
		Owner:      controllerName(p),
		Containers: cs.Names(),
		Init:       cs.InitNames(),
		State:      PodStateOf(p),
	}
}

// statusReason names a pod's state the way kubectl's STATUS column does
// for finished pods: the pod's reason (Evicted), else the first terminated
// container's reason, else the phase.
func statusReason(p *corev1.Pod) string {
	if p.Status.Reason != "" {
		return p.Status.Reason
	}
	for _, cs := range p.Status.ContainerStatuses {
		if t := cs.State.Terminated; t != nil && t.Reason != "" {
			return t.Reason
		}
	}
	return string(p.Status.Phase)
}

func controllerName(p *corev1.Pod) string {
	ref := metav1.GetControllerOfNoCopy(p)
	if ref == nil {
		return ""
	}
	return ref.Kind + "/" + ref.Name
}
