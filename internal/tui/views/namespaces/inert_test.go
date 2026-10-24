package namespaces_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The row shows the cleaned name and phase; what the row selects keeps
// the raw name, read from the listed namespace and never back from a
// cell. A cleaned name would switch to another, real namespace.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	raw := tuitest.Hostile("shop")
	ns := tuitest.Namespace(raw)
	ns.Status.Phase = corev1.NamespacePhase(tuitest.Hostile("Terminating"))
	cases := []struct {
		name string
		keys []string
	}{
		{name: "enter selects the raw name", keys: []string{"down", "enter"}},
		{name: "a filter on the shown name finds it", keys: []string{"/", "s", "h", "o", "p", "enter", "down", "enter"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(ns))
			m := mounted(t, c)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range []string{"shop", "Terminating"} {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}
			m, msgs := tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			tuitest.AssertInert(t, tuitest.Frame(m))
			want := views.NamespaceSelectedMsg{Namespace: raw}
			if !tuitest.Contains(msgs, want) {
				t.Errorf("%v sent %v, want the raw %+v", tc.keys, msgs, want)
			}
		})
	}
}

// Selected and State name the raw namespace under the cursor.
func TestHostileSelectionStaysRaw(t *testing.T) {
	t.Parallel()

	raw := tuitest.Hostile("shop")
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(raw)))
	m, _ := tuitest.Drive(t, mounted(t, c), tuitest.Press("down"))
	if got, ok := m.Selected(); !ok || got != (views.ResourceRef{Kind: "namespace", Name: raw}) {
		t.Errorf("Selected() = %+v, %v, want the raw name", got, ok)
	}
	if got := m.State().Selected; got != views.RowKey("", raw) {
		t.Errorf("State().Selected = %q, want the raw row key", got)
	}
}
