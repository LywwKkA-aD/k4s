/*
This file ports the READY, STATUS and RESTARTS logic of printPod from
k8s.io/kubernetes/pkg/printers/internalversion/printers.go (v1.36.0), the
code behind the columns of `kubectl get pods`.

Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Changes: the internal API types became k8s.io/api/core/v1, the function was
reduced to the three columns and split into helpers, and the clock for
"(5m ago)" is passed in.
*/

package k8s

import (
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

// nodeLostReason is the pod status.reason the node lifecycle controller
// sets when the node stops reporting (node.NodeUnreachablePodReason).
const nodeLostReason = "NodeLost"

// PodColumns holds the READY, STATUS and RESTARTS cells `kubectl get pods`
// prints for one pod.
type PodColumns struct {
	// Ready is "ready/total"; native sidecars (restartable init
	// containers) count in both.
	Ready string
	// Status is kubectl's STATUS: Running, Init:1/2, Init:CrashLoopBackOff,
	// Evicted, Terminating, Completed and so on.
	Status string
	// Restarts counts init container restarts while the pod initializes and
	// sidecar plus app container restarts afterwards.
	Restarts int32
	// LastRestart is when the most recent counted restart happened; zero
	// when no counted container has terminated before.
	LastRestart time.Time
}

// RestartsText renders RESTARTS as kubectl does at now: "3 (5m ago)", or
// the bare count when there is no restart time.
func (c PodColumns) RestartsText(now time.Time) string {
	if c.Restarts == 0 || c.LastRestart.IsZero() {
		return strconv.Itoa(int(c.Restarts))
	}
	return fmt.Sprintf("%d (%s ago)", c.Restarts, duration.HumanDuration(now.Sub(c.LastRestart)))
}

// PodColumnsFor computes kubectl's READY, STATUS and RESTARTS for pod.
func PodColumnsFor(pod *corev1.Pod) PodColumns {
	sidecars := sidecarNames(pod.Spec.InitContainers)
	t := podTally{
		reason: podReason(pod.Status),
		total:  len(pod.Spec.Containers) + countSidecars(pod.Spec.InitContainers),
	}
	t = t.withInitContainers(pod, sidecars)
	if !t.initializing || isPodInitialized(pod.Status) {
		t = t.withContainers(pod.Status)
	}
	return PodColumns{
		Ready:       fmt.Sprintf("%d/%d", t.ready, t.total),
		Status:      deletionStatus(pod, t.reason),
		Restarts:    t.restarts,
		LastRestart: t.lastRestart,
	}
}

// podTally is the running state of the printPod walk; each step returns an
// updated copy.
type podTally struct {
	reason       string
	ready        int
	total        int
	restarts     int32
	lastRestart  time.Time
	initializing bool

	// Sidecar restarts replace the init total once the pod is initialized.
	sidecarRestarts    int32
	sidecarLastRestart time.Time
}

// withInitContainers walks init container statuses in order and stops at
// the first one that still blocks the pod, which then names the status.
func (t podTally) withInitContainers(pod *corev1.Pod, sidecars map[string]bool) podTally {
	for i := range pod.Status.InitContainerStatuses {
		cs := &pod.Status.InitContainerStatuses[i]
		isSidecar := sidecars[cs.Name]
		last := lastTermination(cs)
		t.restarts += cs.RestartCount
		t.lastRestart = later(t.lastRestart, last)
		if isSidecar {
			t.sidecarRestarts += cs.RestartCount
			t.sidecarLastRestart = later(t.sidecarLastRestart, last)
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0 {
			continue
		}
		if isSidecar && cs.Started != nil && *cs.Started {
			if cs.Ready {
				t.ready++
			}
			continue
		}
		t.reason = blockingInitReason(cs, i, len(pod.Spec.InitContainers))
		t.initializing = true
		return t
	}
	return t
}

// blockingInitReason is the STATUS for the i-th of n init containers when
// it has not finished.
func blockingInitReason(cs *corev1.ContainerStatus, i, n int) string {
	switch {
	case cs.State.Terminated != nil:
		return terminatedReason(cs.State.Terminated, "Init:")
	case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing":
		return "Init:" + cs.State.Waiting.Reason
	default:
		return fmt.Sprintf("Init:%d/%d", i, n)
	}
}

// withContainers walks app container statuses last to first, so the first
// container's state wins, as in kubectl.
func (t podTally) withContainers(status corev1.PodStatus) podTally {
	t.restarts = t.sidecarRestarts
	t.lastRestart = t.sidecarLastRestart
	hasRunning := false
	errorReason := ""
	for i := len(status.ContainerStatuses) - 1; i >= 0; i-- {
		cs := &status.ContainerStatuses[i]
		t.restarts += cs.RestartCount
		t.lastRestart = later(t.lastRestart, lastTermination(cs))
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			t.reason = cs.State.Waiting.Reason
		case cs.State.Terminated != nil:
			t.reason = terminatedReason(cs.State.Terminated, "")
			if cs.State.Terminated.ExitCode != 0 {
				errorReason = t.reason
			}
		case cs.Ready && cs.State.Running != nil:
			hasRunning = true
			t.ready++
		}
	}
	t.reason = completedReason(t.reason, hasRunning, errorReason, status.Conditions)
	return t
}

// completedReason turns "Completed" back into Running or NotReady while a
// container still runs, or into the error of a container that failed.
func completedReason(reason string, hasRunning bool, errorReason string, conds []corev1.PodCondition) string {
	if reason != "Completed" {
		return reason
	}
	switch {
	case hasRunning && hasCondition(conds, corev1.PodReady):
		return "Running"
	case errorReason != "":
		return errorReason
	case hasRunning:
		return "NotReady"
	}
	return reason
}

func terminatedReason(term *corev1.ContainerStateTerminated, prefix string) string {
	switch {
	case term.Reason != "":
		return prefix + term.Reason
	case term.Signal != 0:
		return fmt.Sprintf("%sSignal:%d", prefix, term.Signal)
	default:
		return fmt.Sprintf("%sExitCode:%d", prefix, term.ExitCode)
	}
}

// podReason is the starting STATUS: the phase, replaced by status.reason
// (Evicted, NodeLost, ...) and by a scheduling gate.
func podReason(status corev1.PodStatus) string {
	reason := string(status.Phase)
	if status.Reason != "" {
		reason = status.Reason
	}
	for _, c := range status.Conditions {
		if c.Type == corev1.PodScheduled && c.Reason == corev1.PodReasonSchedulingGated {
			reason = corev1.PodReasonSchedulingGated
		}
	}
	return reason
}

func deletionStatus(pod *corev1.Pod, reason string) string {
	if pod.DeletionTimestamp == nil {
		return reason
	}
	if pod.Status.Reason == nodeLostReason {
		return "Unknown"
	}
	if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
		return "Terminating"
	}
	return reason
}

func isRestartable(c *corev1.Container) bool {
	return c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

func countSidecars(initContainers []corev1.Container) int {
	n := 0
	for i := range initContainers {
		if isRestartable(&initContainers[i]) {
			n++
		}
	}
	return n
}

func sidecarNames(initContainers []corev1.Container) map[string]bool {
	out := make(map[string]bool, len(initContainers))
	for i := range initContainers {
		if isRestartable(&initContainers[i]) {
			out[initContainers[i].Name] = true
		}
	}
	return out
}

func lastTermination(cs *corev1.ContainerStatus) time.Time {
	if cs.LastTerminationState.Terminated == nil {
		return time.Time{}
	}
	return cs.LastTerminationState.Terminated.FinishedAt.Time
}

func later(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

func isPodInitialized(status corev1.PodStatus) bool {
	for _, c := range status.Conditions {
		if c.Type == corev1.PodInitialized {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func hasCondition(conds []corev1.PodCondition, condType corev1.PodConditionType) bool {
	for _, c := range conds {
		if c.Type == condType && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
