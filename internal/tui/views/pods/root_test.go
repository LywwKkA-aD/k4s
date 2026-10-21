package pods_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// These tests drive the root model, since Esc back from the logs crosses
// it: the root takes the pods view's State, builds a fresh view on Esc and
// restores it (M3.9).

const (
	shop  = "shop"
	rootW = 200
	rootH = 40
)

// shopObjects sort as cart-0, pay-api-0, pay-api-1, pay-worker-0, web-0:
// "pay" matches the middle three.
func shopObjects() []runtime.Object {
	names := []string{"cart-0", "pay-api-0", "pay-api-1", "pay-worker-0", "web-0"}
	objs := make([]runtime.Object, 0, len(names)+1)
	objs = append(objs, tuitest.Namespace(shop))
	for _, name := range names {
		objs = append(objs, tuitest.Pod(shop, name, nil, tuitest.ContainerApp))
	}
	return objs
}

func rootLines(m tui.Model) []string {
	return strings.Split(tuitest.Screen(m), "\n")
}

func rootView(m tui.Model) string {
	_, after, _ := strings.Cut(rootLines(m)[0], "view: ")
	return strings.TrimSpace(after)
}

func TestEscFromLogsBringsThePodsBack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		extra []tea.Msg
		hint  string
	}{
		{name: "watching", hint: "w watch 5s"},
		{name: "paused", extra: tuitest.Keys("w"), hint: "w paused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
			m, _ := tuitest.Mount(t, tui.New(c, nil), rootW, rootH)
			m, _ = tuitest.Drive(t, m, views.NamespaceSelectedMsg{Namespace: shop})
			m, _ = tuitest.Drive(t, m, tuitest.FilterBy("pay")...)
			m, _ = tuitest.Drive(t, m, down(2)...)
			m, _ = tuitest.Drive(t, m, tc.extra...)
			before := tuitest.Footer(m)
			if !strings.Contains(before, tc.hint) {
				t.Fatalf("footer before lacks %q:\n%s", tc.hint, before)
			}
			tuitest.Golden(t, "root_footer_"+tc.name, before+"\n")

			m, out := tuitest.Drive(t, m, tuitest.Press("l"), tuitest.Enter())
			if _, ok := tuitest.Find[views.LogsRequestMsg](out); !ok || rootView(m) == "pods" {
				t.Fatalf("l, Enter did not open the logs (view %q):\n%s", rootView(m), tuitest.Describe(out))
			}

			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			if got := rootView(m); got != "pods" {
				t.Fatalf("Esc from the logs went to %q", got)
			}
			screen := tuitest.Screen(m)
			if !strings.Contains(screen, "/ pay") || strings.Contains(screen, "cart-0") {
				t.Errorf("the filter did not come back:\n%s", screen)
			}
			if after := tuitest.Footer(m); after != before {
				t.Errorf("footer after Esc:\n%s\nwant as before:\n%s", after, before)
			}

			_, out = tuitest.Drive(t, m, tuitest.Enter())
			want := views.DescribeRequestMsg{Kind: "pod", Namespace: shop, Name: "pay-worker-0"}
			if !tuitest.Contains(out, want) {
				t.Errorf("the cursor left the third match; enter asked:\n%s", tuitest.Describe(tuitest.Requests(out)))
			}
		})
	}
}

// TestEscClearsTheFilterBeforeLeaving: the first Esc clears a committed
// filter and stays; the next one goes back (M3.6).
func TestEscClearsTheFilterBeforeLeaving(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	m, _ := tuitest.Mount(t, tui.New(c, nil), rootW, rootH)
	m, _ = tuitest.Drive(t, m, views.NamespaceSelectedMsg{Namespace: shop})
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("pay")...)

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if got := rootView(m); got != "pods" {
		t.Fatalf("the first Esc left the view for %q", got)
	}
	if screen := tuitest.Screen(m); strings.Contains(screen, "/ pay") || !strings.Contains(screen, "cart-0") {
		t.Errorf("the first Esc kept the filter:\n%s", screen)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if got := rootView(m); got != "dashboard" {
		t.Errorf("the second Esc went to %q, want the dashboard", got)
	}
}
