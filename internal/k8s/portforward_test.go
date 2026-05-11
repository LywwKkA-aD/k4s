package k8s

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// All resolver tests target a single "demo" namespace; the helpers
// hardcode it so the call sites stay short. Add an ns-parameterised
// variant later if anything actually needs cross-namespace coverage.
const testNamespace = "demo"

func mkService(name string, selector map[string]string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       corev1.ServiceSpec{Selector: selector, Ports: ports},
	}
}

func mkPod(name string, phase corev1.PodPhase, ready bool, labels map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace, Labels: labels},
		Status:     corev1.PodStatus{Phase: phase},
	}
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	} else {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
	}
	return pod
}

func TestStartPodPortForwardRefusesWhenRestConfigNil(t *testing.T) {
	t.Parallel()
	c := &Client{Clientset: fake.NewSimpleClientset(), RestConfig: nil}
	_, err := c.StartPodPortForward(context.Background(), "ns", "pod", 8080, 80)
	if err == nil {
		t.Fatal("expected error when RestConfig is nil")
	}
	if !strings.Contains(err.Error(), "rest config") {
		t.Errorf("error %q should mention rest config", err.Error())
	}
}

func TestResolveServicePortPicksFirstByDefault(t *testing.T) {
	t.Parallel()
	svc := mkService("nginx", map[string]string{"app": "nginx"},
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
		corev1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
	)
	got, err := resolveServicePort(svc, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 8080 {
		t.Errorf("default port = %d, want 8080 (targetPort of first)", got)
	}
}

func TestResolveServicePortByName(t *testing.T) {
	t.Parallel()
	svc := mkService("nginx", map[string]string{"app": "nginx"},
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
		corev1.ServicePort{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
	)
	got, err := resolveServicePort(svc, "https")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 8443 {
		t.Errorf("named port https = %d, want 8443", got)
	}
}

func TestResolveServicePortUnknownNameErrors(t *testing.T) {
	t.Parallel()
	svc := mkService("nginx", map[string]string{"app": "nginx"},
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
	)
	_, err := resolveServicePort(svc, "ghost")
	if err == nil {
		t.Error("expected error on unknown port name")
	}
}

func TestResolveServicePortNoPorts(t *testing.T) {
	t.Parallel()
	svc := mkService("naked", map[string]string{"app": "x"})
	_, err := resolveServicePort(svc, "")
	if err == nil {
		t.Error("expected error when service has no ports")
	}
}

func TestResolveServicePortFallsBackToPortWhenNoTargetPort(t *testing.T) {
	t.Parallel()
	// targetPort=0 → fall back to spec.Port. Real-world manifests omit
	// targetPort all the time; behaviour must match kubectl.
	svc := mkService("nginx", map[string]string{"app": "nginx"},
		corev1.ServicePort{Name: "http", Port: 80},
	)
	got, err := resolveServicePort(svc, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 80 {
		t.Errorf("fallback = %d, want 80", got)
	}
}

func TestFirstReadyPodSkipsPendingAndNotReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		*mkPod("a", corev1.PodPending, false, nil),
		*mkPod("b", corev1.PodRunning, false, nil),
		*mkPod("c", corev1.PodRunning, true, nil),
		*mkPod("d", corev1.PodRunning, true, nil),
	}
	got, ok := firstReadyPod(pods)
	if !ok {
		t.Fatal("expected a ready pod")
	}
	if got != "c" {
		t.Errorf("got %q, want c (first running+ready)", got)
	}
}

func TestFirstReadyPodReturnsFalseWhenNoneReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		*mkPod("a", corev1.PodPending, false, nil),
		*mkPod("b", corev1.PodRunning, false, nil),
	}
	_, ok := firstReadyPod(pods)
	if ok {
		t.Error("expected false when no pod is ready")
	}
}

func TestResolveServiceToPodHappyPath(t *testing.T) {
	t.Parallel()
	svc := mkService("nginx", map[string]string{"app": "nginx"},
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
	)
	objs := []runtime.Object{
		svc,
		mkPod("nginx-aaa", corev1.PodRunning, false, map[string]string{"app": "nginx"}),
		mkPod("nginx-bbb", corev1.PodRunning, true, map[string]string{"app": "nginx"}),
	}
	cs := fake.NewSimpleClientset(objs...)
	c := &Client{Clientset: cs}

	pod, port, err := c.ResolveServiceToPod(context.Background(), "demo", "nginx", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod != "nginx-bbb" {
		t.Errorf("pod = %q, want nginx-bbb (the ready one)", pod)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
}

func TestResolveServiceToPodErrorsOnSelectorless(t *testing.T) {
	t.Parallel()
	svc := mkService("naked", nil)
	cs := fake.NewSimpleClientset(svc)
	c := &Client{Clientset: cs}
	_, _, err := c.ResolveServiceToPod(context.Background(), "demo", "naked", "")
	if err == nil {
		t.Error("expected error on selector-less service")
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Errorf("error %q should mention selector", err.Error())
	}
}

func TestResolveDeploymentToPodHappyPath(t *testing.T) {
	t.Parallel()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "log-spammer", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "log-spammer"}},
		},
	}
	objs := []runtime.Object{
		dep,
		mkPod("log-spammer-1", corev1.PodRunning, true, map[string]string{"app": "log-spammer"}),
	}
	cs := fake.NewSimpleClientset(objs...)
	c := &Client{Clientset: cs}
	pod, err := c.ResolveDeploymentToPod(context.Background(), "demo", "log-spammer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod != "log-spammer-1" {
		t.Errorf("pod = %q, want log-spammer-1", pod)
	}
}

func TestResolveDeploymentNotFoundErrors(t *testing.T) {
	t.Parallel()
	cs := fake.NewSimpleClientset()
	c := &Client{Clientset: cs}
	_, err := c.ResolveDeploymentToPod(context.Background(), "demo", "ghost")
	if err == nil {
		t.Error("expected error for missing deployment")
	}
}
