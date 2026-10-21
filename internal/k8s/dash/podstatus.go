package dash

import (
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// podReason approximates the STATUS word kubectl prints, enough to tell
// same-named pods apart. The full printPod port belongs to M4.5.
func podReason(p corev1.Pod) string {
	if p.Status.Reason != "" {
		return p.Status.Reason
	}
	for i, c := range p.Status.InitContainerStatuses {
		switch {
		case c.State.Terminated != nil && c.State.Terminated.ExitCode == 0, sidecarRunning(p, c):
			continue
		case c.State.Terminated != nil:
			return "Init:" + terminatedReason(c.State.Terminated)
		case c.State.Waiting != nil && c.State.Waiting.Reason != "" && c.State.Waiting.Reason != "PodInitializing":
			return "Init:" + c.State.Waiting.Reason
		default:
			return "Init:" + strconv.Itoa(i) + "/" + strconv.Itoa(len(p.Spec.InitContainers))
		}
	}
	reason := string(p.Status.Phase)
	for _, c := range p.Status.ContainerStatuses {
		switch {
		case c.State.Waiting != nil && c.State.Waiting.Reason != "":
			reason = c.State.Waiting.Reason
		case c.State.Terminated != nil:
			reason = terminatedReason(c.State.Terminated)
		}
	}
	if reason == string(corev1.PodSucceeded) {
		return "Completed"
	}
	return reason
}

func terminatedReason(t *corev1.ContainerStateTerminated) string {
	switch {
	case t.Reason != "":
		return t.Reason
	case t.ExitCode == 0:
		return "Completed"
	default:
		return "Error"
	}
}

// sidecarRunning reports a native sidecar (an init container with
// restartPolicy Always) that has started; it does not block the pod.
func sidecarRunning(p corev1.Pod, c corev1.ContainerStatus) bool {
	for _, ic := range p.Spec.InitContainers {
		if ic.Name == c.Name && ic.RestartPolicy != nil && *ic.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			return c.Started != nil && *c.Started
		}
	}
	return false
}

func readyCounts(p corev1.Pod) (ready, total int) {
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
	}
	return ready, len(p.Spec.Containers)
}

func restartsOf(p corev1.Pod) int {
	n := 0
	for _, c := range p.Status.ContainerStatuses {
		n += int(c.RestartCount)
	}
	for _, c := range p.Status.InitContainerStatuses {
		n += int(c.RestartCount)
	}
	return n
}

// crashContainer names the container a logs command must target: the init
// container that blocks the pod, else the container that is crashing or
// was terminated, else the one restarting most, else the first one.
func crashContainer(p corev1.Pod) (string, bool) {
	for _, c := range p.Status.InitContainerStatuses {
		if sidecarRunning(p, c) {
			continue
		}
		if failingStatus(c) {
			return c.Name, true
		}
	}
	for _, c := range p.Status.ContainerStatuses {
		if failingStatus(c) {
			return c.Name, false
		}
	}
	best, most := "", int32(-1)
	for _, c := range p.Status.ContainerStatuses {
		if c.RestartCount > most {
			best, most = c.Name, c.RestartCount
		}
	}
	if best == "" && len(p.Spec.Containers) > 0 {
		best = p.Spec.Containers[0].Name
	}
	return best, false
}

func failingStatus(c corev1.ContainerStatus) bool {
	if w := c.State.Waiting; w != nil {
		return slices.Contains(crashReasons, w.Reason) || slices.Contains(pullReasons, w.Reason) || slices.Contains(configReasons, w.Reason)
	}
	if t := c.State.Terminated; t != nil {
		return t.ExitCode != 0
	}
	return false
}

// resolvedPod is what a typed lookup keeps of one pod.
func resolvedPod(p corev1.Pod) resolvedObj {
	ready, total := readyCounts(p)
	container, init := crashContainer(p)
	return resolvedObj{
		namespace: p.Namespace,
		node:      p.Spec.NodeName,
		class:     classOf(podReason(p), ready, total, true),
		restarts:  restartsOf(p),
		container: container,
		init:      init,
	}
}
