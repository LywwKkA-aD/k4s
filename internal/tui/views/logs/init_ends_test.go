package logs

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// initPod is a pod held back by its init containers: migrate runs, the
// native sidecar log-shipper runs beside it, and app waits.
func initPod() *corev1.Pod {
	always := corev1.ContainerRestartPolicyAlways
	p := webPod("web-h1-a", "h1", 0)
	p.Spec.InitContainers = []corev1.Container{{Name: "migrate"}, {Name: "log-shipper", RestartPolicy: &always}}
	p.Status.Phase = corev1.PodPending
	p.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}
	run := corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}}
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "migrate", State: run}, {Name: "log-shipper", State: run}}
	return p
}

// TestInitContainerEndsSayWhy: the stream of an init container or a
// native sidecar ends with the same notices an app container's does,
// read from its own status, and an init container that ran to completion
// ends for good.
func TestInitContainerEndsSayWhy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		container string
		status    func(*corev1.ContainerStatus)
		want      string
		final     bool
	}{
		{
			name: "an init container completed", container: "migrate",
			status: func(cs *corev1.ContainerStatus) {
				cs.State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}
			},
			want: "[web-h1-a] ○ ended: init container completed (exit 0)", final: true,
		},
		{
			name: "an init container failed", container: "migrate",
			status: func(cs *corev1.ContainerStatus) {
				cs.State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}
			},
			want: "[web-h1-a] ◇ container exited (Error, exit 1) · waiting for a restart",
		},
		{
			name: "a sidecar restarted", container: "log-shipper",
			status: func(cs *corev1.ContainerStatus) {
				cs.RestartCount = 1
				cs.LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}
			},
			want: "[web-h1-a] ◇ container restarted (OOMKilled, exit 137) · reattached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stestClient(initPod())
			r := mountContainer(t, c, cs, tc.container, []string{"web-h1-a"})
			r.mutate("web-h1-a", func(p *corev1.Pod) {
				for i := range p.Status.InitContainerStatuses {
					if p.Status.InitContainerStatuses[i].Name == tc.container {
						tc.status(&p.Status.InitContainerStatuses[i])
					}
				}
			})
			r.feed(r.end("web-h1-a", nil))
			r.advance(explainDebounce)
			r.shows(tc.want)
			if !tc.final {
				return
			}
			r.advance(time.Minute)
			if r.src.count("web-h1-a") != 1 || r.clock.Pending() != 0 {
				t.Errorf("a completed init container reconnects: %d streams, %d timers", r.src.count("web-h1-a"), r.clock.Pending())
			}
		})
	}
}
