package k8s

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListPodsRendersStatusReadinessAndRestarts(t *testing.T) {
	t.Parallel()

	twoMinAgo := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	// Hours, so a slow test run cannot tip "(3h ago)" over to "(3h1m ago)".
	threeHoursAgo := metav1.NewTime(time.Now().Add(-3 * time.Hour))

	objs := []runtime.Object{
		// healthy pod, 1/1 ready
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "happy", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}, NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.1",
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "main", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
		},
		// crashlooping pod, 0/1 ready, 5 restarts
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "main",
						Ready:        false,
						RestartCount: 5,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
						},
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{FinishedAt: threeHoursAgo},
						},
					},
				},
			},
		},
		// multi-container, partial readiness
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "web"}, {Name: "side"},
			}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "web", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					{Name: "side", Ready: false, RestartCount: 1, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
		},
		// init container failing: kubectl shows the init state, not the phase
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "init-crash", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{Name: "migrate"}},
				Containers:     []corev1.Container{{Name: "main"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{{
					Name:         "migrate",
					RestartCount: 4,
					State:        corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{FinishedAt: threeHoursAgo},
					},
				}},
			},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}

	got, err := c.ListPods(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListPods error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 pods, got %d", len(got))
	}

	byName := map[string]Pod{}
	for _, p := range got {
		byName[p.Name] = p
	}

	cases := []struct {
		name         string
		wantReady    string
		wantStatus   string
		wantRestarts int32
		wantText     string
	}{
		{"happy", "1/1", "Running", 0, "0"},
		{"broken", "0/1", "CrashLoopBackOff", 5, "5 (3h ago)"},
		{"multi", "1/2", "Running", 1, "1"},
		{"init-crash", "0/1", "Init:CrashLoopBackOff", 4, "4 (3h ago)"},
	}

	for _, tc := range cases {
		p, ok := byName[tc.name]
		if !ok {
			t.Errorf("pod %q missing from result", tc.name)
			continue
		}
		if p.Ready != tc.wantReady {
			t.Errorf("%s Ready = %q, want %q", tc.name, p.Ready, tc.wantReady)
		}
		if p.Status != tc.wantStatus {
			t.Errorf("%s Status = %q, want %q", tc.name, p.Status, tc.wantStatus)
		}
		if p.Restarts != tc.wantRestarts {
			t.Errorf("%s Restarts = %d, want %d", tc.name, p.Restarts, tc.wantRestarts)
		}
		if p.RestartsText != tc.wantText {
			t.Errorf("%s RestartsText = %q, want %q", tc.name, p.RestartsText, tc.wantText)
		}
	}
}

func TestContainersForPodReturnsAllContainers(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", Namespace: "demo"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "web"},
			{Name: "tailer"},
			{Name: "metrics"},
		}},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(pod)), Context: "test"}

	got, err := c.ContainersForPod(context.Background(), "demo", "multi")
	if err != nil {
		t.Fatalf("ContainersForPod: %v", err)
	}
	want := []string{"web", "tailer", "metrics"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContainersForPodMissing(t *testing.T) {
	t.Parallel()
	c := &Client{Clientset: fake.NewSimpleClientset(), Context: "test"}
	_, err := c.ContainersForPod(context.Background(), "demo", "ghost")
	if err == nil {
		t.Error("expected error for missing pod, got nil")
	}
}

func TestListPodsAllNamespaces(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "a"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "b"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}

	got, err := c.ListPods(context.Background(), "")
	if err != nil {
		t.Fatalf("ListPods error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 pods across all namespaces, got %d", len(got))
	}
}
