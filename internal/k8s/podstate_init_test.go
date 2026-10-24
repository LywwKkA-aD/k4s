package k8s_test

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// TestPodStateOfReadsInitContainers: the init containers and native
// sidecars stand after the app containers, read from their own statuses,
// so a stream of one is explained as an app container's is. The default
// container stays the first app container.
func TestPodStateOfReadsInitContainers(t *testing.T) {
	t.Parallel()

	start := time.Unix(1_700_000_500, 0).UTC()
	always := corev1.ContainerRestartPolicyAlways
	pod := podWith(corev1.PodPending, corev1.ContainerStatus{
		Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}},
	})
	pod.Spec.InitContainers = []corev1.Container{{Name: "migrate"}, {Name: "log-shipper", RestartPolicy: &always}, {Name: "seed"}}
	crashing := corev1.ContainerStatus{
		Name: "migrate", RestartCount: 3,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
	}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{crashing, running("log-shipper", 1, start)}

	cases := []struct {
		container string
		want      k8s.ContainerState
	}{
		{container: "", want: k8s.ContainerState{Name: "app", Waiting: "PodInitializing"}},
		{
			container: "migrate",
			want:      k8s.ContainerState{Name: "migrate", Init: true, Restarts: 3, Waiting: "CrashLoopBackOff", LastExit: &k8s.ContainerExit{Reason: "Error", Code: 1}},
		},
		{container: "log-shipper", want: k8s.ContainerState{Name: "log-shipper", Init: true, Sidecar: true, Restarts: 1, Running: true, Started: start}},
		{container: "seed", want: k8s.ContainerState{Name: "seed", Init: true, Waiting: string(corev1.PodPending)}},
	}
	s := k8s.PodStateOf(pod)
	for _, tc := range cases {
		t.Run("container "+tc.container, func(t *testing.T) {
			t.Parallel()
			got, ok := s.Container(tc.container)
			if !ok || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Container(%q) = %+v, %v\nwant %+v", tc.container, got, ok, tc.want)
			}
		})
	}
}
