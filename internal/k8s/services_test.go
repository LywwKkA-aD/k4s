package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListServicesRendersClusterIPType(t *testing.T) {
	t.Parallel()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "demo", CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute))},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.5",
			Ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
				{Port: 443, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(svc)), Context: "test"}

	got, err := c.ListServices(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 service, got %d", len(got))
	}
	s := got[0]
	if s.Type != "ClusterIP" || s.ClusterIP != "10.0.0.5" || s.ExternalIP != "<none>" {
		t.Errorf("unexpected service shape: %+v", s)
	}
	if s.Ports != "80/TCP,443/TCP" {
		t.Errorf("ports = %q, want %q", s.Ports, "80/TCP,443/TCP")
	}
}

func TestListServicesRendersHeadlessServiceClusterIP(t *testing.T) {
	t.Parallel()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "headless", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Ports:     []corev1.ServicePort{{Port: 9000, Protocol: corev1.ProtocolTCP}},
		},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(svc)), Context: "test"}

	got, _ := c.ListServices(context.Background(), "demo")
	if got[0].ClusterIP != "<none>" {
		t.Errorf("headless cluster IP = %q, want <none>", got[0].ClusterIP)
	}
}

func TestListServicesRendersNodePortAndLoadBalancer(t *testing.T) {
	t.Parallel()

	nodePort := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.0.0.6",
			Ports:     []corev1.ServicePort{{Port: 80, NodePort: 31000, Protocol: corev1.ProtocolTCP}},
		},
	}
	pendingLB := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-pending", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.7",
			Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
		},
		// no Status.LoadBalancer.Ingress → pending
	}
	readyLB := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-ready", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeLoadBalancer,
			ClusterIP: "10.0.0.8",
			Ports:     []corev1.ServicePort{{Port: 80, NodePort: 31001, Protocol: corev1.ProtocolTCP}},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}, {Hostname: "host.example"}},
			},
		},
	}
	externalName := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ext", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "db.example.com",
		},
	}

	c := &Client{
		Clientset: fake.NewSimpleClientset(
			runtime.Object(nodePort),
			runtime.Object(pendingLB),
			runtime.Object(readyLB),
			runtime.Object(externalName),
		),
		Context: "test",
	}

	got, _ := c.ListServices(context.Background(), "demo")

	by := map[string]Service{}
	for _, s := range got {
		by[s.Name] = s
	}
	if got := by["np"]; got.Ports != "80:31000/TCP" || got.ExternalIP != "<none>" {
		t.Errorf("nodeport: %+v", got)
	}
	if got := by["lb-pending"]; got.ExternalIP != "<pending>" {
		t.Errorf("lb-pending external IP = %q, want <pending>", got.ExternalIP)
	}
	if got := by["lb-ready"]; got.ExternalIP != "1.2.3.4,host.example" || !strings.Contains(got.Ports, ":31001") {
		t.Errorf("lb-ready: %+v", got)
	}
	if got := by["ext"]; got.ExternalIP != "db.example.com" {
		t.Errorf("ExternalName external IP = %q, want db.example.com", got.ExternalIP)
	}
}

func TestListServicesEmptyPortsRendersAsNone(t *testing.T) {
	t.Parallel()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "noports", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.9",
		},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(svc)), Context: "test"}

	got, _ := c.ListServices(context.Background(), "demo")
	if got[0].Ports != "<none>" {
		t.Errorf("ports = %q, want <none>", got[0].Ports)
	}
}

func TestPodsAndContainersForServiceHappyPath(t *testing.T) {
	t.Parallel()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "demo"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "nginx"}},
	}
	pods := []runtime.Object{
		svc,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-a", Namespace: "demo", Labels: map[string]string{"app": "nginx"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "web"},
				{Name: "sidecar"},
			}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-b", Namespace: "demo", Labels: map[string]string{"app": "nginx"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}, {Name: "sidecar"}}},
		},
		// Should not match — different label.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "demo", Labels: map[string]string{"app": "other"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "x"}}},
		},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(pods...)}

	gotPods, gotContainers, err := c.PodsAndContainersForService(context.Background(), "demo", "nginx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotPods) != 2 {
		t.Errorf("expected 2 pods, got %d: %v", len(gotPods), gotPods)
	}
	if len(gotContainers) != 2 {
		t.Errorf("expected 2 containers, got %d: %v", len(gotContainers), gotContainers)
	}
}

func TestPodsAndContainersForServiceSelectorlessReturnsNil(t *testing.T) {
	t.Parallel()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "headless", Namespace: "demo"},
		Spec:       corev1.ServiceSpec{}, // no selector
	}
	c := &Client{Clientset: fake.NewSimpleClientset(svc)}
	pods, containers, err := c.PodsAndContainersForService(context.Background(), "demo", "headless")
	if err != nil {
		t.Fatalf("selector-less service should not error, got %v", err)
	}
	if pods != nil || containers != nil {
		t.Errorf("expected (nil, nil), got pods=%v containers=%v", pods, containers)
	}
}

func TestPodsAndContainersForServiceNoMatchingPods(t *testing.T) {
	t.Parallel()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lonely", Namespace: "demo"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "ghost"}},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(svc)}
	pods, containers, err := c.PodsAndContainersForService(context.Background(), "demo", "lonely")
	if err != nil {
		t.Fatalf("no-match case should not error, got %v", err)
	}
	if pods != nil || containers != nil {
		t.Errorf("expected (nil, nil), got pods=%v containers=%v", pods, containers)
	}
}

func TestPodsAndContainersForServiceMissingServiceErrors(t *testing.T) {
	t.Parallel()
	c := &Client{Clientset: fake.NewSimpleClientset()}
	_, _, err := c.PodsAndContainersForService(context.Background(), "demo", "absent")
	if err == nil {
		t.Errorf("expected error for missing service")
	}
}
