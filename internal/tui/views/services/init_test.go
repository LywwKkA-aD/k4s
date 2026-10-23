package services_test

import (
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestLogsPickerOffersInitContainers: l on a service offers what l on its
// first pod does, the init containers and native sidecars after the app
// containers, named in Init so the picker labels them init:. An init
// container next to a single app container opens the picker too.
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
			containers: []string{tuitest.ContainerNginx, "migrate", "log-shipper"},
			initNames:  []string{"migrate", "log-shipper"},
		},
		{
			name:       "one app container and one init container",
			init:       []corev1.Container{{Name: "migrate"}},
			containers: []string{tuitest.ContainerNginx, "migrate"},
			initNames:  []string{"migrate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			labels := map[string]string{"app": "migrator"}
			pod := tuitest.Pod(shop, "migrator-1", labels, tuitest.ContainerNginx)
			pod.Spec.InitContainers = tc.init
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(shop), tuitest.Service(shop, "migrator", 8080, labels), pod))
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
