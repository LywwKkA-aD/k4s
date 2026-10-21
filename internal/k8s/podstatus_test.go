/*
Most fixtures below are ported from TestPrintPod and
TestPrintPodWithRestartableInitContainer in
k8s.io/kubernetes/pkg/printers/internalversion/printers_test.go (v1.36.0).

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

Changes: the internal API types became k8s.io/api/core/v1, the clock is
fixed instead of time.Now, and k4s cases (eviction, node loss, init
failures, signals) were added.
*/

package k8s

import (
	"reflect"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var podNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) metav1.Time { return metav1.NewTime(podNow.Add(-d)) }

func running() corev1.ContainerState {
	return corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
}

func waiting(reason string) corev1.ContainerState {
	return corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}
}

func terminated(reason string, code int32) corev1.ContainerState {
	return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: reason, ExitCode: code}}
}

// lastExit is a LastTerminationState that finished d before podNow.
func lastExit(d time.Duration) corev1.ContainerState {
	return corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{FinishedAt: ago(d)}}
}

func containers(n int) []corev1.Container { return make([]corev1.Container, n) }

func sidecar(name string) corev1.Container {
	always := corev1.ContainerRestartPolicyAlways
	return corev1.Container{Name: name, RestartPolicy: &always}
}

func boolPtr(b bool) *bool { return &b }

type podColumnsCase struct {
	name         string
	pod          corev1.Pod
	wantReady    string
	wantStatus   string
	wantRestarts string
}

func upstreamPodCases() []podColumnsCase {
	deleted := ago(10 * time.Second)
	return []podColumnsCase{
		{
			name: "restarts summed, only running ready containers count",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: "podPhase", ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 3, State: running()},
					{RestartCount: 3},
				}},
			},
			wantReady: "1/2", wantStatus: "podPhase", wantRestarts: "6",
		},
		{
			name: "container waiting reason overrides the phase",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: "podPhase", ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 3, State: running()},
					{State: waiting("ContainerWaitingReason"), RestartCount: 3},
				}},
			},
			wantReady: "1/2", wantStatus: "ContainerWaitingReason", wantRestarts: "6",
		},
		{
			name: "the first container's reason wins",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: "podPhase", ContainerStatuses: []corev1.ContainerStatus{
					{State: waiting("ContainerWaitingReason"), RestartCount: 3},
					{State: terminated("ContainerTerminatedReason", 0), RestartCount: 3},
				}},
			},
			wantReady: "0/2", wantStatus: "ContainerWaitingReason", wantRestarts: "6",
		},
		{
			name: "pod reason overrides the phase",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Reason: "podReason", Phase: "podPhase", ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 3, State: running()},
					{Ready: true, RestartCount: 3},
				}},
			},
			wantReady: "1/2", wantStatus: "podReason", wantRestarts: "6",
		},
		{
			name: "completed plus running without a ready condition is NotReady",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 3, State: terminated("Completed", 0)},
					{Ready: true, RestartCount: 3, State: running()},
				}},
			},
			wantReady: "1/2", wantStatus: "NotReady", wantRestarts: "6",
		},
		{
			name: "completed plus running with a ready condition is Running",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true, RestartCount: 3, State: terminated("Completed", 0)},
						{Ready: true, RestartCount: 3, State: running()},
					},
					Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				},
			},
			wantReady: "1/2", wantStatus: "Running", wantRestarts: "6",
		},
		{
			name: "init container restarting",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 "podPhase",
					InitContainerStatuses: []corev1.ContainerStatus{{RestartCount: 3, State: running(), LastTerminationState: lastExit(10 * time.Second)}},
					ContainerStatuses:     []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:0/1", wantRestarts: "3 (10s ago)",
		},
		{
			name: "first of two init containers restarting",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(2), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: "podPhase",
					InitContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 3, State: running(), LastTerminationState: lastExit(10 * time.Second)},
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
					},
					ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:0/2", wantRestarts: "3 (10s ago)",
		},
		{
			name: "second init container restarting after the first completed",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(2), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: "podPhase",
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
						{RestartCount: 3, State: running(), LastTerminationState: lastExit(10 * time.Second)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:1/2", wantRestarts: "3 (10s ago)",
		},
		{
			name: "init restarts add up and the latest restart is shown",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(2), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: "podPhase",
					InitContainerStatuses: []corev1.ContainerStatus{
						{RestartCount: 2, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}, LastTerminationState: lastExit(2 * time.Minute)},
						{RestartCount: 3, State: running(), LastTerminationState: lastExit(10 * time.Second)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:1/2", wantRestarts: "5 (10s ago)",
		},
		{
			name: "finished init restarts drop out once the pod runs",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{{RestartCount: 2, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}, LastTerminationState: lastExit(2 * time.Minute)}},
					ContainerStatuses:     []corev1.ContainerStatus{{RestartCount: 4, State: running(), LastTerminationState: lastExit(20 * time.Second)}},
				},
			},
			wantReady: "0/1", wantStatus: "Running", wantRestarts: "4 (20s ago)",
		},
		{
			name: "restart days ago",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 3, State: running(), LastTerminationState: lastExit(5 * 24 * time.Hour)},
				}},
			},
			wantReady: "1/1", wantStatus: "Running", wantRestarts: "3 (5d ago)",
		},
		{
			name: "latest restart across containers",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, RestartCount: 6, State: running(), LastTerminationState: lastExit(5 * 24 * time.Hour)},
					{Ready: true, RestartCount: 3, State: running(), LastTerminationState: lastExit(20 * 24 * time.Hour)},
				}},
			},
			wantReady: "2/2", wantStatus: "Running", wantRestarts: "9 (5d ago)",
		},
		{
			name: "scheduling gated",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: "podPhase", Conditions: []corev1.PodCondition{
					{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonSchedulingGated},
				}},
			},
			wantReady: "0/2", wantStatus: "SchedulingGated", wantRestarts: "0",
		},
		{
			name: "succeeded pod is Completed",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodSucceeded, ContainerStatuses: []corev1.ContainerStatus{{State: terminated("Completed", 0)}}},
			},
			wantReady: "0/1", wantStatus: "Completed", wantRestarts: "0",
		},
		{
			name: "failed pod shows the container reason",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodFailed, ContainerStatuses: []corev1.ContainerStatus{{State: terminated("Error", 1)}}},
			},
			wantReady: "0/1", wantStatus: "Error", wantRestarts: "0",
		},
		{
			name: "a deleted pod that already succeeded is not Terminating",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleted},
				Spec:       corev1.PodSpec{Containers: containers(1)},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded, ContainerStatuses: []corev1.ContainerStatus{{State: terminated("Completed", 0)}}},
			},
			wantReady: "0/1", wantStatus: "Completed", wantRestarts: "0",
		},
		{
			name: "a deleted running pod is Terminating",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleted},
				Spec:       corev1.PodSpec{Containers: containers(1)},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{State: running()}}},
			},
			wantReady: "0/1", wantStatus: "Terminating", wantRestarts: "0",
		},
		{
			name: "a deleted pending pod is Terminating",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleted},
				Spec:       corev1.PodSpec{Containers: containers(1)},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			wantReady: "0/1", wantStatus: "Terminating", wantRestarts: "0",
		},
		{
			name: "error then running keeps the error (kubernetes#107713)",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: corev1.PodFailed, ContainerStatuses: []corev1.ContainerStatus{
					{State: terminated("podCrash", 1)},
					{Ready: true, State: running()},
				}},
			},
			wantReady: "1/2", wantStatus: "podCrash", wantRestarts: "0",
		},
		{
			name: "running, error and completed show the error",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(3)},
				Status: corev1.PodStatus{Phase: corev1.PodFailed, ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, State: running()},
					{Ready: true, State: terminated("podCrash", 1)},
					{Ready: true, State: terminated("", 0)},
				}},
			},
			wantReady: "1/3", wantStatus: "podCrash", wantRestarts: "0",
		},
	}
}

func sidecarPodCases() []podColumnsCase {
	twoSidecars := []corev1.Container{sidecar("sidecar-1"), sidecar("sidecar-2")}
	return []podColumnsCase{
		{
			name: "sidecar running but not started",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: twoSidecars, Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "sidecar-1", RestartCount: 3, State: running(), Started: boolPtr(false), LastTerminationState: lastExit(10 * time.Second)},
						{Name: "sidecar-2", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}, Started: boolPtr(false)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
					Conditions:        []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionFalse}},
				},
			},
			wantReady: "0/3", wantStatus: "Init:0/2", wantRestarts: "3 (10s ago)",
		},
		{
			name: "first sidecar started, second not yet",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: twoSidecars, Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "sidecar-1", RestartCount: 3, State: running(), Started: boolPtr(true), LastTerminationState: lastExit(10 * time.Second)},
						{Name: "sidecar-2", State: running(), Started: boolPtr(false)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}}},
					Conditions:        []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionFalse}},
				},
			},
			wantReady: "0/3", wantStatus: "Init:1/2", wantRestarts: "3 (10s ago)",
		},
		{
			name: "sidecar restarts count once the pod runs",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: twoSidecars, Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "sidecar-1", RestartCount: 3, State: running(), Started: boolPtr(true), LastTerminationState: lastExit(10 * time.Second)},
						{Name: "sidecar-2", State: running(), Started: boolPtr(true)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 4, State: running(), LastTerminationState: lastExit(20 * time.Second)}},
					Conditions:        []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionTrue}},
				},
			},
			wantReady: "1/3", wantStatus: "Running", wantRestarts: "7 (10s ago)",
		},
		{
			name: "ready sidecars count in READY",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "setup"}, sidecar("proxy")}, Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "setup", State: terminated("Completed", 0)},
						{Name: "proxy", Ready: true, State: running(), Started: boolPtr(true)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{Ready: true, State: running()}},
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodInitialized, Status: corev1.ConditionTrue},
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			wantReady: "2/2", wantStatus: "Running", wantRestarts: "0",
		},
		{
			name: "sidecars killed after the main container completed",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: twoSidecars, Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "sidecar-1", RestartCount: 3, State: terminated("Error", 137), Started: boolPtr(false), LastTerminationState: lastExit(10 * time.Second)},
						{Name: "sidecar-2", State: terminated("Error", 137), Started: boolPtr(false)},
					},
					ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 4, State: terminated("Completed", 0), LastTerminationState: lastExit(20 * time.Second)}},
					Conditions:        []corev1.PodCondition{{Type: corev1.PodInitialized, Status: corev1.ConditionTrue}},
				},
			},
			wantReady: "0/3", wantStatus: "Completed", wantRestarts: "7 (10s ago)",
		},
		{
			name: "statuses for containers missing from the spec",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "init1"}, sidecar("sidecar1")},
					Containers:     []corev1.Container{{Name: "container1"}},
				},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{{Name: "initinvalid"}, {Name: "init1"}, {Name: "sidecar1"}},
					ContainerStatuses:     []corev1.ContainerStatus{{Name: "containerinvalid"}, {Name: "container1"}},
				},
			},
			wantReady: "0/2", wantStatus: "Init:0/2", wantRestarts: "0",
		},
	}
}

func k4sPodCases() []podColumnsCase {
	deleted := ago(time.Minute)
	return []podColumnsCase{
		{
			name: "evicted pod shows the pod reason",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted", ContainerStatuses: []corev1.ContainerStatus{
					{State: terminated("ContainerStatusUnknown", 137)},
				}},
			},
			// The container status still wins, exactly as in kubectl.
			wantReady: "0/1", wantStatus: "ContainerStatusUnknown", wantRestarts: "0",
		},
		{
			name: "evicted pod without container statuses",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"},
			},
			wantReady: "0/1", wantStatus: "Evicted", wantRestarts: "0",
		},
		{
			name: "deleted pod on a lost node is Unknown",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleted},
				Spec:       corev1.PodSpec{Containers: containers(1)},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, Reason: "NodeLost", ContainerStatuses: []corev1.ContainerStatus{{Ready: true, State: running()}}},
			},
			wantReady: "1/1", wantStatus: "Unknown", wantRestarts: "0",
		},
		{
			name: "pod on a lost node, not yet deleted",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, Reason: "NodeLost"},
			},
			wantReady: "0/1", wantStatus: "NodeLost", wantRestarts: "0",
		},
		{
			name: "init container in crash loop",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{{RestartCount: 33, State: waiting("CrashLoopBackOff"), LastTerminationState: lastExit(6*time.Minute + 42*time.Second)}},
					ContainerStatuses:     []corev1.ContainerStatus{{State: waiting("PodInitializing")}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:CrashLoopBackOff", wantRestarts: "33 (6m42s ago)",
		},
		{
			name: "init container failed with a reason",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{{RestartCount: 1, State: terminated("Error", 1), LastTerminationState: lastExit(3 * time.Hour)}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:Error", wantRestarts: "1 (3h ago)",
		},
		{
			name: "init container killed by a signal",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Signal: 9, ExitCode: 137}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:Signal:9", wantRestarts: "0",
		},
		{
			name: "init container exited without a reason",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(1), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2}}}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:ExitCode:2", wantRestarts: "0",
		},
		{
			name: "init container still waiting to initialize",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: containers(2), Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{{State: waiting("PodInitializing")}},
				},
			},
			wantReady: "0/1", wantStatus: "Init:0/2", wantRestarts: "0",
		},
		{
			name: "container killed by a signal",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Signal: 15, ExitCode: 143}}},
				}},
			},
			wantReady: "0/1", wantStatus: "Signal:15", wantRestarts: "0",
		},
		{
			name: "container exited without a reason",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 3}}},
				}},
			},
			wantReady: "0/1", wantStatus: "ExitCode:3", wantRestarts: "0",
		},
		{
			name: "crash loop with a recent restart",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 38, State: waiting("CrashLoopBackOff"), LastTerminationState: lastExit(96 * time.Second)},
				}},
			},
			wantReady: "0/1", wantStatus: "CrashLoopBackOff", wantRestarts: "38 (96s ago)",
		},
		{
			name: "restarts without a termination time print the bare count",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 2, State: running()}}},
			},
			wantReady: "1/1", wantStatus: "Running", wantRestarts: "2",
		},
		{
			name:      "pod with no status yet",
			pod:       corev1.Pod{Spec: corev1.PodSpec{Containers: containers(1)}},
			wantReady: "0/1", wantStatus: "", wantRestarts: "0",
		},
	}
}

func TestPodColumnsFor(t *testing.T) {
	t.Parallel()

	cases := slices.Concat(upstreamPodCases(), sidecarPodCases(), k4sPodCases())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PodColumnsFor(&tc.pod)
			if got.Ready != tc.wantReady || got.Status != tc.wantStatus {
				t.Errorf("READY/STATUS = %q/%q, want %q/%q", got.Ready, got.Status, tc.wantReady, tc.wantStatus)
			}
			if r := got.RestartsText(podNow); r != tc.wantRestarts {
				t.Errorf("RESTARTS = %q, want %q", r, tc.wantRestarts)
			}
		})
	}
}

func TestPodColumnsForDoesNotMutate(t *testing.T) {
	t.Parallel()

	pod := sidecarPodCases()[2].pod
	before := pod.DeepCopy()
	_ = PodColumnsFor(&pod)
	if !reflect.DeepEqual(before, &pod) {
		t.Fatal("PodColumnsFor changed its input")
	}
}
