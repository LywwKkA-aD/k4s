package k8s_test

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// withInit gives spec a plain init container, migrate, and a native
// sidecar, log-shipper, after its app container.
func withInit(spec *corev1.PodSpec) {
	always := corev1.ContainerRestartPolicyAlways
	spec.InitContainers = []corev1.Container{{Name: "migrate"}, {Name: "log-shipper", RestartPolicy: &always}}
}

// TestLogTargetsListInitContainers: a deployment's or a service's logs
// offer what a pod's do, the app containers then the init containers and
// native sidecars, with Init naming the latter so the picker labels them.
// Every pod streamed carries the same names, so a pod that joins keeps
// an init container the user picked, and a re-resolve keeps them.
func TestLogTargetsListInitContainers(t *testing.T) {
	t.Parallel()

	wantContainers := []string{"app", "migrate", "log-shipper"}
	wantInit := []string{"migrate", "log-shipper"}
	dep := deployment("shop", "web", "uid-web", map[string]string{"app": "web"})
	withInit(&dep.Spec.Template.Spec)
	pod := rsPod("shop", "web-h1-a", "web", "h1", "uid-rs", corev1.PodRunning)
	withInit(&pod.Spec)
	objs := []runtime.Object{dep, replicaSet(dep, "h1", "uid-rs", "1", 1), pod, service("shop", "web", map[string]string{"app": "web"})}

	cases := []struct {
		name string
		kind string
	}{
		{name: "deployment", kind: k8s.LogKindDeployment},
		{name: "service", kind: k8s.LogKindService},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(objs...)
			target, err := c.ResolveLogTarget(context.Background(), tc.kind, "shop", "web")
			if err != nil {
				t.Fatalf("ResolveLogTarget: %v", err)
			}
			again, err := c.ReresolveLogTarget(context.Background(), target)
			if err != nil {
				t.Fatalf("ReresolveLogTarget: %v", err)
			}
			for _, got := range []k8s.LogTarget{target, again} {
				if !slices.Equal(got.Containers, wantContainers) || !slices.Equal(got.Init, wantInit) {
					t.Errorf("Containers = %q, Init = %q; want %q, %q", got.Containers, got.Init, wantContainers, wantInit)
				}
				if len(got.Pods) != 1 || !slices.Equal(got.Pods[0].Containers, wantContainers) {
					t.Errorf("pods = %+v, want web-h1-a with %q", got.Pods, wantContainers)
				}
			}
		})
	}
}
