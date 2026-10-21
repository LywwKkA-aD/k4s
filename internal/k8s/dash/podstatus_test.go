package dash

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func waiting(name, reason string, restarts int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: name, RestartCount: restarts, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}
}

func running(name string) corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: name, Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
}

func terminated(name string, exit int32, reason string) corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: name, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: exit, Reason: reason}}}
}

func podOf(init, main []corev1.ContainerStatus) corev1.Pod {
	p := corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, InitContainerStatuses: init, ContainerStatuses: main}}
	for _, c := range init {
		p.Spec.InitContainers = append(p.Spec.InitContainers, corev1.Container{Name: c.Name})
	}
	for _, c := range main {
		p.Spec.Containers = append(p.Spec.Containers, corev1.Container{Name: c.Name})
	}
	return p
}

func TestCrashContainerPicksTheLogsTarget(t *testing.T) {
	t.Parallel()

	always := corev1.ContainerRestartPolicyAlways
	started := true
	sidecar := podOf([]corev1.ContainerStatus{{Name: "mesh", Started: &started, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
		[]corev1.ContainerStatus{running("app"), waiting("worker", "CrashLoopBackOff", 3)})
	sidecar.Spec.InitContainers[0].RestartPolicy = &always

	cases := []struct {
		name  string
		pod   corev1.Pod
		want  string
		init  bool
		state string
	}{
		{"the crashing one of two", podOf(nil, []corev1.ContainerStatus{running("loki"), waiting("ingester", "CrashLoopBackOff", 12)}), "ingester", false, "CrashLoopBackOff"},
		{"an init container blocks first", podOf([]corev1.ContainerStatus{waiting("migrate-db", "CrashLoopBackOff", 5)}, []corev1.ContainerStatus{waiting("app", "PodInitializing", 0)}), "migrate-db", true, "Init:CrashLoopBackOff"},
		{"a failed init container", podOf([]corev1.ContainerStatus{terminated("setup", 1, "")}, []corev1.ContainerStatus{waiting("app", "PodInitializing", 0)}), "setup", true, "Init:Error"},
		{"a native sidecar does not block", sidecar, "worker", false, "CrashLoopBackOff"},
		{"terminated with an error", podOf(nil, []corev1.ContainerStatus{running("a"), terminated("b", 137, "OOMKilled")}), "b", false, "OOMKilled"},
		{"else the one restarting most", podOf(nil, []corev1.ContainerStatus{{Name: "a", RestartCount: 1}, {Name: "b", RestartCount: 4}}), "b", false, "Running"},
		{"else the first container", corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "only"}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}}, "only", false, "Pending"},
		{"an init container still to run", podOf([]corev1.ContainerStatus{waiting("init", "PodInitializing", 0), waiting("init2", "", 0)}, nil), "", false, "Init:0/2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, init := crashContainer(tc.pod)
			if name != tc.want || init != tc.init {
				t.Fatalf("crashContainer = %q init=%v, want %q init=%v", name, init, tc.want, tc.init)
			}
			if got := podReason(tc.pod); got != tc.state {
				t.Fatalf("podReason = %q, want %q", got, tc.state)
			}
		})
	}
}

func TestPodReasonEdgeCases(t *testing.T) {
	t.Parallel()

	evicted := corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"}}
	done := podOf(nil, []corev1.ContainerStatus{terminated("app", 0, "")})
	done.Status.Phase = corev1.PodSucceeded
	initDone := podOf([]corev1.ContainerStatus{terminated("init", 0, "Completed")}, []corev1.ContainerStatus{running("app")})
	for _, tc := range []struct {
		pod  corev1.Pod
		want string
	}{{evicted, "Evicted"}, {done, "Completed"}, {initDone, "Running"}} {
		if got := podReason(tc.pod); got != tc.want {
			t.Errorf("podReason = %q, want %q", got, tc.want)
		}
	}
	if got := resolvedPod(initDone); got.class != ClassOK || got.restarts != 0 {
		t.Errorf("resolvedPod = %+v", got)
	}
	if replicas(nil) != 1 {
		t.Error("spec.replicas defaults to 1")
	}
}
