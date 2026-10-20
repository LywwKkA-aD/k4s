package forwards

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const resolveNS = "k4s-demo"

func readyPod(name string, labels map[string]string, ports ...corev1.ContainerPort) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: resolveNS, Labels: labels},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Ports: ports}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

// resolveFixture is a Traefik-shaped Service (two ports, named targetPorts)
// plus a Deployment, both backed by the same ready pod.
func resolveFixture() *Manager {
	labels := map[string]string{"app": "named-port"}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "named-port", Namespace: resolveNS},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromString("web")},
				{Name: "https", Port: 8443, TargetPort: intstr.FromString("websecure")},
			},
		},
	}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "named-port", Namespace: resolveNS},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: labels}},
	}
	pod := readyPod("named-port-abc", labels,
		corev1.ContainerPort{Name: "web", ContainerPort: 8080},
		corev1.ContainerPort{Name: "websecure", ContainerPort: 9443},
	)
	client := &k8s.Client{Clientset: fake.NewSimpleClientset(svc, dep, pod)}
	return &Manager{client: client, active: map[string]*Active{}, changeCh: make(chan struct{})}
}

func TestResolveTargetHonoursRemotePort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fwd      Forward
		wantPod  string
		wantPort uint16
		wantErr  string
	}{
		{
			name:    "pod uses the remote port as typed",
			fwd:     Forward{Kind: "pod", Name: "some-pod", RemotePort: 5432},
			wantPod: "some-pod", wantPort: 5432,
		},
		{
			name:    "deployment uses the remote port as typed",
			fwd:     Forward{Kind: "deployment", Name: "named-port", RemotePort: 8080},
			wantPod: "named-port-abc", wantPort: 8080,
		},
		{
			name:    "service first port resolves its named targetPort",
			fwd:     Forward{Kind: "service", Name: "named-port", RemotePort: 80},
			wantPod: "named-port-abc", wantPort: 8080,
		},
		{
			name:    "service second port is not replaced by the first",
			fwd:     Forward{Kind: "service", Name: "named-port", RemotePort: 8443},
			wantPod: "named-port-abc", wantPort: 9443,
		},
		{
			name:    "service port that does not exist is an error",
			fwd:     Forward{Kind: "service", Name: "named-port", RemotePort: 8080},
			wantErr: `service "named-port" has no port 8080 (ports: 80, 8443)`,
		},
		{
			name:    "unknown kind",
			fwd:     Forward{Kind: "statefulset", Name: "x", RemotePort: 80},
			wantErr: `unknown kind "statefulset"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := resolveFixture()
			fwd := tt.fwd
			fwd.Namespace = resolveNS
			pod, port, err := m.resolveTarget(context.Background(), fwd)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got pod=%q port=%d", tt.wantErr, pod, port)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pod != tt.wantPod {
				t.Errorf("pod = %q, want %q", pod, tt.wantPod)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestPortNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      int32
		want    uint16
		wantErr bool
	}{
		{name: "lowest", in: 1, want: 1},
		{name: "highest", in: 65535, want: 65535},
		{name: "zero", in: 0, wantErr: true},
		{name: "negative", in: -1, wantErr: true},
		{name: "too large", in: 65536, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := portNumber(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("portNumber(%d) = %d, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("portNumber(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
