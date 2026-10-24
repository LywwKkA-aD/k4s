package k8s

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// defaultContainerAnnotation names the container kubectl logs and exec pick
// when no -c is given.
const defaultContainerAnnotation = "kubectl.kubernetes.io/default-container"

// PodState is a pod as its log streams see it: whether it is going away,
// how it ended, and where each container stands. The logs view tells
// from it why a stream ended and when a waiting pod has a log to follow.
// The reasons are inert text (see internal/sanitize): the node and the
// controllers write them, and the view prints them.
type PodState struct {
	Phase string
	// Reason is the pod's own reason, such as Evicted.
	Reason string
	// Deleting is set once the pod has a deletion timestamp: its streams
	// are about to end for good.
	Deleting bool
	// DefaultContainer is the kubectl.kubernetes.io/default-container
	// annotation, empty when unset.
	DefaultContainer string
	// Containers are the app containers, then the init containers and
	// native sidecars, each in spec order.
	Containers []ContainerState
}

// ContainerState is one container's status.
type ContainerState struct {
	Name string
	// Init marks an init container, a native sidecar included; Sidecar
	// marks a native sidecar, which runs beside the app containers. A
	// plain init container runs to completion before they start.
	Init     bool
	Sidecar  bool
	Restarts int32
	// Waiting is why the container waits (ContainerCreating,
	// CrashLoopBackOff, or Unschedulable for a pod not yet placed).
	Waiting string
	// Running is set while the current instance runs; Started is when it
	// started, zero when the status does not say.
	Running bool
	Started time.Time
	// Exit is how the current instance ended; nil unless it has.
	Exit *ContainerExit
	// LastExit is how the instance before it ended; nil before a restart.
	LastExit *ContainerExit
}

// ContainerExit is how a container instance ended.
type ContainerExit struct {
	Reason string
	Code   int32
}

// HasLog reports whether the container has run, so its log can be read:
// it runs, has ended, or has restarted before.
func (c ContainerState) HasLog() bool {
	return c.Running || c.Exit != nil || c.LastExit != nil || c.Restarts > 0
}

// Finished reports a pod that ran to completion or failed: its log is
// over.
func (s PodState) Finished() bool {
	return s.Phase == string(corev1.PodSucceeded) || s.Phase == string(corev1.PodFailed)
}

// Container returns the named container; "" means the one kubectl logs
// picks without -c, the default-container annotation or else the first
// app container.
func (s PodState) Container(name string) (ContainerState, bool) {
	if name == "" {
		name = s.DefaultContainer
	}
	for _, c := range s.Containers {
		if name == "" || c.Name == name {
			return c, true
		}
	}
	return ContainerState{}, false
}

// PodState reads the named pod's state with one GET. A missing pod is the
// API's NotFound error.
func (c *Client) PodState(ctx context.Context, namespace, name string) (PodState, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return PodState{}, fmt.Errorf("get pod: %w", err)
	}
	return PodStateOf(pod), nil
}

// PodStateOf reads p's state: the app containers from their statuses,
// then the init containers and native sidecars from theirs. A container
// without a status yet waits for what holds the pod back: its
// scheduling, or "Pending".
func PodStateOf(p *corev1.Pod) PodState {
	s := PodState{
		Phase:            string(p.Status.Phase),
		Reason:           sanitize.Line(p.Status.Reason),
		Deleting:         p.DeletionTimestamp != nil,
		DefaultContainer: p.Annotations[defaultContainerAnnotation],
		Containers:       make([]ContainerState, 0, len(p.Spec.Containers)+len(p.Spec.InitContainers)),
	}
	statuses := byContainer(p.Status.ContainerStatuses)
	for _, ct := range p.Spec.Containers {
		s.Containers = append(s.Containers, stateIn(p, ct.Name, statuses))
	}
	initStatuses := byContainer(p.Status.InitContainerStatuses)
	for _, ct := range p.Spec.InitContainers {
		c := stateIn(p, ct.Name, initStatuses)
		c.Init = true
		c.Sidecar = ct.RestartPolicy != nil && *ct.RestartPolicy == corev1.ContainerRestartPolicyAlways
		s.Containers = append(s.Containers, c)
	}
	return s
}

func byContainer(list []corev1.ContainerStatus) map[string]corev1.ContainerStatus {
	out := make(map[string]corev1.ContainerStatus, len(list))
	for _, cs := range list {
		out[cs.Name] = cs
	}
	return out
}

// stateIn is the state of p's container name from statuses, or what holds
// the pod back when it has no status yet.
func stateIn(p *corev1.Pod, name string, statuses map[string]corev1.ContainerStatus) ContainerState {
	cs, ok := statuses[name]
	if !ok {
		return ContainerState{Name: name, Waiting: pendingReason(p)}
	}
	return containerStateOf(cs)
}

func containerStateOf(cs corev1.ContainerStatus) ContainerState {
	out := ContainerState{Name: cs.Name, Restarts: cs.RestartCount, LastExit: exitOf(cs.LastTerminationState.Terminated)}
	switch st := cs.State; {
	case st.Running != nil:
		out.Running, out.Started = true, st.Running.StartedAt.Time
	case st.Terminated != nil:
		out.Exit = exitOf(st.Terminated)
	case st.Waiting != nil:
		out.Waiting = sanitize.Line(st.Waiting.Reason)
	}
	return out
}

func exitOf(t *corev1.ContainerStateTerminated) *ContainerExit {
	if t == nil {
		return nil
	}
	return &ContainerExit{Reason: sanitize.Line(t.Reason), Code: t.ExitCode}
}

// pendingReason is why a pod's containers have no status yet: it is not
// scheduled, or simply Pending; empty once it is past Pending.
func pendingReason(p *corev1.Pod) string {
	if p.Status.Phase != corev1.PodPending {
		return ""
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse && c.Reason != "" {
			return sanitize.Line(c.Reason)
		}
	}
	return string(corev1.PodPending)
}
