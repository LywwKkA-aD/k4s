package deployments_test

import (
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// withInit is owned's deployment with init containers in its template
// and its pods.
func withInit(name string, init ...corev1.Container) []runtime.Object {
	objs := owned(name, 1, appContainer)
	out := make([]runtime.Object, 0, len(objs))
	for _, o := range objs {
		switch o := o.(type) {
		case *appsv1.Deployment:
			d := o.DeepCopy()
			d.Spec.Template.Spec.InitContainers = init
			out = append(out, d)
		case *corev1.Pod:
			p := o.DeepCopy()
			p.Spec.InitContainers = init
			out = append(out, p)
		default:
			out = append(out, o)
		}
	}
	return out
}

// TestLogsPickerOffersInitContainers: l on a deployment offers what l on
// one of its pods does, the init containers and native sidecars after
// the app containers, named in Init so the picker labels them init:.
// An init container next to a single app container opens the picker too.
func TestLogsPickerOffersInitContainers(t *testing.T) {
	t.Parallel()

	always := corev1.ContainerRestartPolicyAlways
	cases := []struct {
		name       string
		init       []corev1.Container
		containers []string
		initNames  []string
	}{
		{
			name:       "an init container and a native sidecar",
			init:       []corev1.Container{{Name: "migrate"}, {Name: "log-shipper", RestartPolicy: &always}},
			containers: []string{appContainer, "migrate", "log-shipper"},
			initNames:  []string{"migrate", "log-shipper"},
		},
		{
			name:       "one app container and one init container",
			init:       []corev1.Container{{Name: "migrate"}},
			containers: []string{appContainer, "migrate"},
			initNames:  []string{"migrate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			objs := append([]runtime.Object{tuitest.Namespace(shop)}, withInit("migrator", tc.init...)...)
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(objs...))
			_, out := tuitest.Drive(t, mounted(t, c, shop), on("migrator", tuitest.Press("l"))...)
			got, ok := tuitest.Find[views.ContainerPromptRequestMsg](out)
			if !ok {
				t.Fatalf("no container picker:\n%s", tuitest.Describe(out))
			}
			if !slices.Equal(got.Containers, tc.containers) || !slices.Equal(got.Init, tc.initNames) {
				t.Errorf("picker Containers = %q, Init = %q; want %q, %q", got.Containers, got.Init, tc.containers, tc.initNames)
			}
		})
	}
}
