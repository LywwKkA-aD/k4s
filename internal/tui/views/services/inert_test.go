package services_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostileService is an ExternalName service whose namespace, name,
// cluster IP and external name carry escape sequences and invisible
// characters.
func hostileService() *corev1.Service {
	s := tuitest.Service(tuitest.Hostile("shop"), tuitest.Hostile("web"), 80, map[string]string{"app": "web"})
	s.Spec.Type = corev1.ServiceTypeExternalName
	s.Spec.ClusterIP = tuitest.Hostile("10.43.0.10")
	s.Spec.ExternalName = tuitest.Hostile("db.example")
	return s
}

// The row shows the cleaned text; what the row acts on keeps the raw
// name, read from the listed service and never back from a cell.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		want      []string
	}{
		{name: "all namespaces", namespace: "", want: []string{"shop", "web", "10.43.0.10", "db.example"}},
		{name: "one namespace", namespace: tuitest.Hostile("shop"), want: []string{"web", "10.43.0.10", "db.example"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(hostileService()))
			m := mounted(t, c, tc.namespace)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}

			raw := views.ResourceRef{Kind: "service", Namespace: tuitest.Hostile("shop"), Name: tuitest.Hostile("web")}
			if got, ok := m.Selected(); !ok || got != raw {
				t.Errorf("Selected() = %+v, %v, want the raw %+v", got, ok, raw)
			}
			if got := m.State().Selected; got != views.RowKey(raw.Namespace, raw.Name) {
				t.Errorf("State().Selected = %q, want the raw row key", got)
			}
			_, msgs := tuitest.Drive(t, m, tuitest.Press("enter"))
			want := views.DescribeRequestMsg{Kind: "service", Namespace: raw.Namespace, Name: raw.Name}
			if !tuitest.Contains(msgs, want) {
				t.Errorf("enter sent %v, want the raw %+v", msgs, want)
			}
			_, msgs = tuitest.Drive(t, m, tuitest.Press("f"))
			fwd := views.ForwardRequestMsg{Kind: "service", Namespace: raw.Namespace, Name: raw.Name, RemotePort: 80}
			if !tuitest.Contains(msgs, fwd) {
				t.Errorf("f sent %v, want the raw %+v", msgs, fwd)
			}
		})
	}
}
