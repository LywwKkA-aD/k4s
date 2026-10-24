package pods_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostilePod is a crash-looping pod whose namespace, name and waiting
// reason carry escape sequences and invisible characters.
func hostilePod() *corev1.Pod {
	p := tuitest.Pod(tuitest.Hostile("shop"), tuitest.Hostile("web-1"), nil, "app")
	p.Status.ContainerStatuses[0].Ready = false
	p.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: tuitest.Hostile("CrashLoop")},
	}
	return p
}

// The row shows the cleaned text; what the row acts on keeps the raw
// name, so a request reaches this pod and no other. Cleaning the name
// would aim describe at a real pod "web-1" instead.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		want      []string
	}{
		{name: "all namespaces", namespace: "", want: []string{"shop", "web-1", "CrashLoop"}},
		{name: "one namespace", namespace: tuitest.Hostile("shop"), want: []string{"web-1", "CrashLoop"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(hostilePod()))
			m := mounted(t, c, tc.namespace)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}

			raw := podRef(tuitest.Hostile("shop"), tuitest.Hostile("web-1"))
			if got, ok := m.Selected(); !ok || got != raw {
				t.Errorf("Selected() = %+v, %v, want the raw %+v", got, ok, raw)
			}
			_, msgs := tuitest.Drive(t, m, tuitest.Press("enter"))
			want := views.DescribeRequestMsg{Kind: "pod", Namespace: raw.Namespace, Name: raw.Name}
			if !tuitest.Contains(msgs, want) {
				t.Errorf("enter sent %v, want the raw %+v", msgs, want)
			}
			if cmd := views.LogsCommand(raw.Namespace, []string{raw.Name}, "", 100, k8s.LogTarget{}); !cmd.IsZero() {
				t.Errorf("logs command for the raw name = %q, want none", cmd.Display())
			}
		})
	}
}
