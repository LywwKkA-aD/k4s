package k8s

import (
	"context"
	"fmt"
	"net/http"
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

func TestFirstReadyPodSkipsPendingAndNotReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		*mkPod("a", corev1.PodPending, false, nil),
		*mkPod("b", corev1.PodRunning, false, nil),
		*mkPod("c", corev1.PodRunning, true, nil),
		*mkPod("d", corev1.PodRunning, true, nil),
	}
	got := firstReadyPod(pods)
	if got == nil {
		t.Fatal("expected a ready pod")
	}
	if got.Name != "c" {
		t.Errorf("got %q, want c (first running+ready)", got.Name)
	}
}

func TestFirstReadyPodReturnsNilWhenNoneReady(t *testing.T) {
	t.Parallel()
	pods := []corev1.Pod{
		*mkPod("a", corev1.PodPending, false, nil),
		*mkPod("b", corev1.PodRunning, false, nil),
	}
	if got := firstReadyPod(pods); got != nil {
		t.Errorf("got %q, want nil when no pod is ready", got.Name)
	}
}

func TestResolveServiceToPodErrorsOnSelectorless(t *testing.T) {
	t.Parallel()
	svc := mkService("naked", nil)
	cs := fake.NewSimpleClientset(svc)
	c := &Client{Clientset: cs}
	_, _, err := c.ResolveServiceToPod(context.Background(), "demo", "naked", 0)
	if err == nil {
		t.Fatal("expected error on selector-less service")
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Errorf("error %q should mention selector", err.Error())
	}
}

// mkReadyPodWithPorts builds a Running+Ready pod labelled app=web whose
// containers expose the given ports, one container per slice.
func mkReadyPodWithPorts(name string, containers ...[]corev1.ContainerPort) *corev1.Pod {
	specs := make([]corev1.Container, 0, len(containers))
	for i, ports := range containers {
		specs = append(specs, corev1.Container{Name: fmt.Sprintf("c%d", i), Ports: ports})
	}
	pod := mkPod(name, corev1.PodRunning, true, map[string]string{"app": "web"})
	pod.Spec = corev1.PodSpec{Containers: specs}
	return pod
}

func TestResolveServiceToPodTranslatesServicePort(t *testing.T) {
	t.Parallel()
	selector := map[string]string{"app": "web"}
	twoIntPorts := mkService("web", selector,
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)},
		corev1.ServicePort{Name: "https", Port: 8443, TargetPort: intstr.FromInt32(9443)},
	)
	twoNamedPorts := mkService("web", selector,
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromString("web")},
		corev1.ServicePort{Name: "https", Port: 8443, TargetPort: intstr.FromString("websecure")},
	)
	namedPod := mkReadyPodWithPorts("web-abc",
		[]corev1.ContainerPort{{Name: "web", ContainerPort: 8080}},
		[]corev1.ContainerPort{{Name: "metrics", ContainerPort: 9100}, {Name: "websecure", ContainerPort: 9443}},
	)
	plainPod := mkReadyPodWithPorts("web-abc")

	tests := []struct {
		name     string
		svc      *corev1.Service
		pods     []runtime.Object
		svcPort  int32
		wantPod  string
		wantPort int32
		wantErr  string
	}{
		{
			name: "int targetPort", svc: twoIntPorts, pods: []runtime.Object{plainPod},
			svcPort: 80, wantPod: "web-abc", wantPort: 8080,
		},
		{
			name: "second of several ports", svc: twoIntPorts, pods: []runtime.Object{plainPod},
			svcPort: 8443, wantPod: "web-abc", wantPort: 9443,
		},
		{
			name: "named targetPort", svc: twoNamedPorts, pods: []runtime.Object{namedPod},
			svcPort: 80, wantPod: "web-abc", wantPort: 8080,
		},
		{
			name: "named targetPort in a later container", svc: twoNamedPorts, pods: []runtime.Object{namedPod},
			svcPort: 8443, wantPod: "web-abc", wantPort: 9443,
		},
		{
			name: "svcPort 0 picks the first declared port", svc: twoIntPorts, pods: []runtime.Object{plainPod},
			svcPort: 0, wantPod: "web-abc", wantPort: 8080,
		},
		{
			name: "empty targetPort falls back to the Service port",
			svc:  mkService("web", selector, corev1.ServicePort{Name: "http", Port: 80}),
			pods: []runtime.Object{plainPod}, svcPort: 80, wantPod: "web-abc", wantPort: 80,
		},
		{
			name: "named targetPort missing on the pod", svc: twoNamedPorts, pods: []runtime.Object{plainPod},
			svcPort: 80, wantErr: `pod "web-abc" has no container port named "web"`,
		},
		{
			name: "port not on the Service lists the ports", svc: twoIntPorts, pods: []runtime.Object{plainPod},
			svcPort: 443, wantErr: `service "web" has no port 443 (ports: 80, 8443)`,
		},
		{
			name: "service without ports", svc: mkService("web", selector), pods: []runtime.Object{plainPod},
			svcPort: 0, wantErr: `service "web" exposes no ports`,
		},
		{
			name: "no ready pod", svc: twoIntPorts,
			pods:    []runtime.Object{mkPod("web-abc", corev1.PodRunning, false, selector)},
			svcPort: 80, wantErr: `no ready pod backs service "web"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			objs := append([]runtime.Object{tt.svc}, tt.pods...)
			c := &Client{Clientset: fake.NewSimpleClientset(objs...)}
			pod, port, err := c.ResolveServiceToPod(context.Background(), testNamespace, "web", tt.svcPort)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got pod=%v port=%d", tt.wantErr, pod, port)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pod == nil || pod.Name != tt.wantPod {
				t.Errorf("pod = %v, want %q", pod, tt.wantPod)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestResolveServiceToPodMissingService(t *testing.T) {
	t.Parallel()
	c := &Client{Clientset: fake.NewSimpleClientset()}
	_, _, err := c.ResolveServiceToPod(context.Background(), testNamespace, "ghost", 80)
	if err == nil || !strings.Contains(err.Error(), "get service") {
		t.Errorf("error = %v, want a wrapped get service error", err)
	}
}

func TestResolveServiceToPodSkipsUnreadyPods(t *testing.T) {
	t.Parallel()
	selector := map[string]string{"app": "web"}
	svc := mkService("web", selector,
		corev1.ServicePort{Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)},
	)
	c := &Client{Clientset: fake.NewSimpleClientset(
		svc,
		mkPod("web-aaa", corev1.PodRunning, false, selector),
		mkPod("web-bbb", corev1.PodRunning, true, selector),
	)}
	pod, _, err := c.ResolveServiceToPod(context.Background(), testNamespace, "web", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod.Name != "web-bbb" {
		t.Errorf("pod = %q, want web-bbb (the ready one)", pod.Name)
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

func TestResolveDeploymentToPodMatchExpressionsSelector(t *testing.T) {
	t.Parallel()
	// A deployment whose selector is expression-only (no matchLabels) used
	// to hit the "no matchLabels selector" error even though the API — and
	// kubectl — accept it just fine.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"demo"}},
				},
			},
		},
	}
	objs := []runtime.Object{
		dep,
		mkPod("demo-1", corev1.PodRunning, true, map[string]string{"app": "demo"}),
	}
	cs := fake.NewSimpleClientset(objs...)
	c := &Client{Clientset: cs}
	pod, err := c.ResolveDeploymentToPod(context.Background(), "demo", "demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod != "demo-1" {
		t.Errorf("pod = %q, want demo-1", pod)
	}
}

func TestSPDYHTTPClientBoundsHeaderWait(t *testing.T) {
	t.Parallel()
	orig := &http.Transport{}
	client := spdyHTTPClient(orig)
	got, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if got.ResponseHeaderTimeout != spdyUpgradeTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", got.ResponseHeaderTimeout, spdyUpgradeTimeout)
	}
	if got == orig {
		t.Error("client must wrap a clone; mutating the caller's transport would leak the timeout elsewhere")
	}
	if orig.ResponseHeaderTimeout != 0 {
		t.Errorf("original transport mutated: ResponseHeaderTimeout = %v, want 0", orig.ResponseHeaderTimeout)
	}
}

func TestSPDYHTTPClientPassesThroughNonTransport(t *testing.T) {
	t.Parallel()
	// Round trippers we don't recognise (wrappers, test doubles) can't be
	// cloned safely — the helper must fall back without panicking.
	stub := stubRoundTripper{}
	client := spdyHTTPClient(stub)
	if client.Transport != stub {
		t.Errorf("transport = %T, want the stub passed through", client.Transport)
	}
}

type stubRoundTripper struct{}

func (stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }
