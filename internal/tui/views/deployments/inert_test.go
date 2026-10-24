package deployments_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The row shows the cleaned namespace and name; describe still asks for
// the raw ones, so it reaches the deployment shown and no other.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	ns, name := tuitest.Hostile("shop"), tuitest.Hostile("api")
	cases := []struct {
		name      string
		namespace string
		want      []string
	}{
		{name: "all namespaces", namespace: "", want: []string{"shop", "api"}},
		{name: "one namespace", namespace: ns, want: []string{"api"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dep := tuitest.Deployment(ns, name, 1, map[string]string{"app": "api"}, "app")
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(ns), dep))
			m := mounted(t, c, tc.namespace)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}
			_, msgs := tuitest.Drive(t, m, tuitest.Enter())
			want := views.DescribeRequestMsg{Kind: "deployment", Namespace: ns, Name: name}
			if !tuitest.Contains(msgs, want) {
				t.Errorf("enter sent %v, want the raw %+v", msgs, want)
			}
		})
	}
}
