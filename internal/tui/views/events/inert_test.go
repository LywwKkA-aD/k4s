package events_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Every cell of an event renders inert, in one namespace and across all of
// them; the selection keeps the event's raw name, and Enter on an object
// whose raw name kubectl would reject describes nothing.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	ns, name := tuitest.Hostile("shop"), tuitest.Hostile("web-1.17f")
	cases := []struct {
		name      string
		namespace string
		want      []string
	}{
		{name: "one namespace", namespace: ns, want: []string{"Warning", "BackOff", "Pod/web-1", "Back-off restarting"}},
		{name: "all namespaces", namespace: "", want: []string{"shop", "BackOff", "Pod/web-1", "Back-off restarting"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := ev(ns, name, "Warning", tuitest.Hostile("BackOff"), tuitest.Hostile("Back-off restarting"),
				pod(ns, tuitest.Hostile("web-1")), 1, time.Hour, time.Hour)
			m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(e)), tc.namespace)

			tuitest.AssertInert(t, tuitest.Frame(m))
			got := strings.Join(rows(m), "\n")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("rows lack the cleaned %q:\n%s", w, got)
				}
			}
			if ref, ok := m.Selected(); !ok || ref != (views.ResourceRef{Kind: "event", Namespace: ns, Name: name}) {
				t.Errorf("Selected() = %+v, %v, want the raw names", ref, ok)
			}
			_, msgs := tuitest.Drive(t, m, tuitest.Enter())
			if req, ok := tuitest.Find[views.DescribeRequestMsg](msgs); ok {
				t.Errorf("enter described %+v, want nothing for a name kubectl rejects", req)
			}
		})
	}
}
