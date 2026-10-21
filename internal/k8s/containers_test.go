package k8s_test

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// crashPod mirrors deploy/seed/70-init-crashloop.yaml and 75-native-sidecar.yaml:
// an app container behind a native sidecar and two plain init containers.
func crashPod() *corev1.Pod {
	always := corev1.ContainerRestartPolicyAlways
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "init-crashloop", Namespace: "k4s-broken"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "log-shipper", RestartPolicy: &always},
				{Name: "check-config"},
				{Name: "migrate"},
			},
			Containers: []corev1.Container{{Name: "app"}, {Name: "metrics"}},
		},
	}
}

func TestPodContainersListsAppThenInitContainers(t *testing.T) {
	t.Parallel()

	c, _ := k8stest.NewFakeClient(crashPod())
	got, err := c.PodContainers(context.Background(), "k4s-broken", "init-crashloop")
	if err != nil {
		t.Fatalf("PodContainers: %v", err)
	}
	want := k8s.Containers{
		{Name: "app"},
		{Name: "metrics"},
		{Name: "log-shipper", Init: true, Sidecar: true},
		{Name: "check-config", Init: true},
		{Name: "migrate", Init: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("PodContainers = %+v\nwant            %+v", got, want)
	}
	if names := got.Names(); !slices.Equal(names, []string{"app", "metrics", "log-shipper", "check-config", "migrate"}) {
		t.Errorf("Names = %v", names)
	}
	if init := got.InitNames(); !slices.Equal(init, []string{"log-shipper", "check-config", "migrate"}) {
		t.Errorf("InitNames = %v", init)
	}
}

func TestContainersForPodIncludesInitContainers(t *testing.T) {
	t.Parallel()

	c, _ := k8stest.NewFakeClient(crashPod())
	got, err := c.ContainersForPod(context.Background(), "k4s-broken", "init-crashloop")
	if err != nil {
		t.Fatalf("ContainersForPod: %v", err)
	}
	if want := []string{"app", "metrics", "log-shipper", "check-config", "migrate"}; !slices.Equal(got, want) {
		t.Fatalf("ContainersForPod = %v, want %v", got, want)
	}
}

func TestPodContainersOfAMissingPod(t *testing.T) {
	t.Parallel()

	c, _ := k8stest.NewFakeClient()
	got, err := c.PodContainers(context.Background(), "demo", "ghost")
	if !apierrors.IsNotFound(err) || got != nil {
		t.Fatalf("PodContainers = %v, %v; want nil and NotFound", got, err)
	}
}

func TestContainersWithoutInit(t *testing.T) {
	t.Parallel()

	var none k8s.Containers
	if none.Names() != nil || none.InitNames() != nil {
		t.Errorf("empty list: Names %v, InitNames %v", none.Names(), none.InitNames())
	}
	plain := k8s.Containers{{Name: "web"}}
	if plain.InitNames() != nil {
		t.Errorf("InitNames of app containers = %v, want nil", plain.InitNames())
	}
}
