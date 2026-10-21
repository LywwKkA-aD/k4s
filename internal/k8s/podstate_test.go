package k8s_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func podWith(phase corev1.PodPhase, statuses ...corev1.ContainerStatus) *corev1.Pod {
	p := ownedPod("shop", "api-1", map[string]string{"app": "api"}, metav1.OwnerReference{}, phase)
	p.Spec.Containers = []corev1.Container{{Name: "app"}, {Name: "proxy"}}
	p.Status.ContainerStatuses = statuses
	return p
}

func running(name string, restarts int32, started time.Time) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: name, RestartCount: restarts,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(started)}},
	}
}

func TestPodStateOf(t *testing.T) {
	t.Parallel()

	start := time.Unix(1_700_000_500, 0).UTC()
	oom := running("app", 2, start)
	oom.LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}
	crashing := corev1.ContainerStatus{
		Name: "app", RestartCount: 4,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
	}
	done := corev1.ContainerStatus{
		Name:  "app",
		State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}},
	}
	creating := corev1.ContainerStatus{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}}
	unscheduled := podWith(corev1.PodPending)
	unscheduled.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable"}}
	evicted := podWith(corev1.PodFailed)
	evicted.Status.Reason = "Evicted"
	deleting := podWith(corev1.PodRunning, running("app", 0, start))
	deleting.DeletionTimestamp = &metav1.Time{Time: start}
	annotated := podWith(corev1.PodRunning, running("app", 0, start), running("proxy", 1, start))
	annotated.Annotations = map[string]string{"kubectl.kubernetes.io/default-container": "proxy"}

	cases := []struct {
		name      string
		pod       *corev1.Pod
		container string
		want      k8s.ContainerState
		check     func(t *testing.T, s k8s.PodState)
	}{
		{
			name: "a running container restarted after an OOM kill", pod: podWith(corev1.PodRunning, oom),
			want: k8s.ContainerState{Name: "app", Restarts: 2, Started: start, LastExit: &k8s.ContainerExit{Reason: "OOMKilled", Code: 137}},
		},
		{
			name: "crash loop back-off keeps the last exit", pod: podWith(corev1.PodRunning, crashing),
			want: k8s.ContainerState{Name: "app", Restarts: 4, Waiting: "CrashLoopBackOff", LastExit: &k8s.ContainerExit{Reason: "Error", Code: 1}},
		},
		{
			name: "a finished container", pod: podWith(corev1.PodSucceeded, done),
			want: k8s.ContainerState{Name: "app", Exit: &k8s.ContainerExit{Reason: "Completed"}},
		},
		{
			name: "a container being created", pod: podWith(corev1.PodPending, creating),
			want: k8s.ContainerState{Name: "app", Waiting: "ContainerCreating"},
		},
		{
			name: "no status yet waits for scheduling", pod: unscheduled,
			want: k8s.ContainerState{Name: "app", Waiting: "Unschedulable"},
		},
		{
			name: "an evicted pod", pod: evicted, want: k8s.ContainerState{Name: "app"},
			check: func(t *testing.T, s k8s.PodState) {
				if s.Reason != "Evicted" || s.Phase != "Failed" || !s.Finished() {
					t.Errorf("evicted = %+v", s)
				}
			},
		},
		{
			name: "a pod being deleted", pod: deleting, want: k8s.ContainerState{Name: "app", Started: start},
			check: func(t *testing.T, s k8s.PodState) {
				if !s.Deleting || s.Finished() {
					t.Errorf("deleting = %+v", s)
				}
			},
		},
		{name: "a named container", pod: annotated, container: "proxy", want: k8s.ContainerState{Name: "proxy", Restarts: 1, Started: start}},
		{name: "the default container annotation", pod: annotated, want: k8s.ContainerState{Name: "proxy", Restarts: 1, Started: start}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := k8s.PodStateOf(tc.pod)
			got, ok := s.Container(tc.container)
			if !ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Container(%q) = %+v, %v\nwant %+v", tc.container, got, ok, tc.want)
			}
			if tc.check != nil {
				tc.check(t, s)
			}
		})
	}
}

func TestContainerStateHasLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		c    k8s.ContainerState
		want bool
	}{
		{name: "running", c: k8s.ContainerState{Started: time.Unix(1, 0)}, want: true},
		{name: "ended", c: k8s.ContainerState{Exit: &k8s.ContainerExit{}}, want: true},
		{name: "crash looping", c: k8s.ContainerState{Waiting: "CrashLoopBackOff", Restarts: 3}, want: true},
		{name: "being created", c: k8s.ContainerState{Waiting: "ContainerCreating"}},
		{name: "unknown", c: k8s.ContainerState{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.c.HasLog(); got != tc.want {
				t.Fatalf("HasLog = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPodStateContainerUnknown(t *testing.T) {
	t.Parallel()

	if _, ok := k8s.PodStateOf(podWith(corev1.PodRunning)).Container("db"); ok {
		t.Error("a container the pod lacks was found")
	}
	if _, ok := (k8s.PodState{}).Container(""); ok {
		t.Error("a pod without containers has a default one")
	}
}

func TestGetPodState(t *testing.T) {
	t.Parallel()

	c, cs := k8stest.NewFakeClient(podWith(corev1.PodRunning, running("app", 1, time.Unix(1_700_000_000, 0))))
	s, err := c.PodState(context.Background(), "shop", "api-1")
	if err != nil {
		t.Fatalf("PodState: %v", err)
	}
	if got, _ := s.Container(""); got.Restarts != 1 || s.Phase != "Running" {
		t.Fatalf("state = %+v", s)
	}
	checkCalls(t, cs.Actions(), []apiCall{{Verb: "get", Resource: "pods"}})

	if _, err := c.PodState(context.Background(), "shop", "gone"); !apierrors.IsNotFound(err) {
		t.Fatalf("missing pod: err = %v, want NotFound", err)
	}
}
