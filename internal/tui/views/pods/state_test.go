package pods_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

var (
	_ views.Stateful    = pods.Model{}
	_ views.Restorer    = pods.Model{}
	_ views.BackHandler = pods.Model{}
)

func key(name string) string { return views.RowKey(demo, name) }

func TestStateReportsHowTheListLooks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []string
		want views.State
	}{
		{name: "fresh", want: views.State{Selected: key(tuitest.PodCrash)}},
		{name: "cursor moved", keys: []string{"down", "down"}, want: views.State{Selected: key(tuitest.PodWeb1), Cursor: 2}},
		{name: "paused", keys: []string{"w"}, want: views.State{Selected: key(tuitest.PodCrash), Paused: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), demo), tuitest.Keys(tc.keys...)...)
			if got := m.State(); got != tc.want {
				t.Errorf("State = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestStateKeepsOnlyACommittedFilter(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), demo)
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("web")...)
	m, _ = tuitest.Drive(t, m, down(1)...)
	want := views.State{Filter: "web", Selected: key(tuitest.PodWeb2), Cursor: 1}
	if got := m.State(); got != want {
		t.Errorf("committed: State = %+v, want %+v", got, want)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("-2"))
	if got := m.State().Filter; got != "" {
		t.Errorf("open filter: State.Filter = %q, want none", got)
	}
}

func TestStateInAllNamespacesKeysByNamespace(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), "")
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy(tuitest.PodCoreDNS)...)
	if got, want := m.State().Selected, views.RowKey(tuitest.NamespaceSystem, tuitest.PodCoreDNS); got != want {
		t.Errorf("Selected = %q, want %q", got, want)
	}
}

// restored builds a fresh view, restores st on it and starts it the way
// the root does: Restore, then Init.
func restored(t *testing.T, c *k8s.Client, st views.State, clk *asynctest.Clock) (pods.Model, []tea.Msg) {
	t.Helper()
	fresh, _ := tuitest.Drive(t, pods.New(c, demo, pods.WithClock(clk)), tuitest.Resize(width, height))
	v, cmd := fresh.Restore(st)
	m, ok := v.(pods.Model)
	if !ok {
		t.Fatalf("Restore returned %T", v)
	}
	if out := tuitest.Collect(t, cmd); len(out) != 0 {
		t.Fatalf("Restore must not fetch or emit, got:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "/ "+st.Filter) && st.Filter != "" {
		t.Errorf("the first frame is not filtered:\n%s", tuitest.Screen(m))
	}
	return tuitest.Init(t, m)
}

func TestRestoreBringsTheListBack(t *testing.T) {
	t.Parallel()

	var log rvLog
	c := tuitest.NewFakeClient(t, log.option())
	clk := asynctest.NewClock(epoch)
	st := views.State{Filter: "web", Selected: key(tuitest.PodWeb2), Cursor: 1, Paused: true}

	m, out := restored(t, c, st, clk)
	if got := m.State(); got != st {
		t.Errorf("State after restore = %+v, want %+v", got, st)
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "/ web") || strings.Contains(screen, tuitest.PodCrash) {
		t.Errorf("the filter is not back:\n%s", screen)
	}
	if reqs := tuitest.Requests(out); len(reqs) != 0 {
		t.Errorf("restore emitted:\n%s", tuitest.Describe(reqs))
	}
	if got := log.all(); len(got) != 1 || got[0] != "0" {
		t.Errorf("restore listed with RVs %q, want one list at \"0\"", got)
	}
	if clk.Pending() != 0 {
		t.Errorf("a paused restore armed %d ticks", clk.Pending())
	}
	if got := watchHint(m); got != "paused" {
		t.Errorf("w hint = %q, want paused", got)
	}
}

func TestRestoreBeforeTheListKeepsTheRow(t *testing.T) {
	t.Parallel()

	st := views.State{Selected: key(tuitest.PodWeb2), Cursor: 3}
	m := newView(tuitest.NewFakeClient(t), demo)
	v, _ := m.Restore(st)
	if got := v.(pods.Model).State(); got != st {
		t.Errorf("State before the first list = %+v, want %+v", got, st)
	}
}

func TestRestoreOfAGoneRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		cursor int
		want   string
	}{
		{name: "lands on the same index", cursor: 2, want: key(tuitest.PodWeb2)},
		{name: "clamped to the list", cursor: 9, want: key(tuitest.PodWeb2)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t)
			if err := c.Clientset.CoreV1().Pods(demo).Delete(context.Background(), tuitest.PodWeb1, metav1.DeleteOptions{}); err != nil {
				t.Fatalf("delete: %v", err)
			}
			st := views.State{Selected: key(tuitest.PodWeb1), Cursor: tc.cursor}
			m, out := restored(t, c, st, asynctest.NewClock(epoch))
			if got := selected(m); got != tc.want {
				t.Errorf("cursor on %q, want %q", got, tc.want)
			}
			want := views.StatusMsg{Level: status.Info, Text: "pod web-1 is gone"}
			if !tuitest.Contains(out, want) {
				t.Errorf("want %+v among:\n%s", want, tuitest.Describe(out))
			}
		})
	}
}

func TestBackClearsACommittedFilter(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), demo)
	if _, _, used := m.Back(); used {
		t.Fatal("Back without a filter must leave Esc to the root")
	}

	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("web")...)
	m, _ = tuitest.Drive(t, m, down(1)...)
	v, cmd, used := m.Back()
	if !used || cmd != nil {
		t.Fatalf("Back with a filter: used=%v cmd=%v", used, cmd != nil)
	}
	back, ok := v.(pods.Model)
	if !ok {
		t.Fatalf("Back returned %T", v)
	}
	if got := back.State(); got.Filter != "" || got.Selected != key(tuitest.PodWeb2) {
		t.Errorf("after Back State = %+v, want no filter and the cursor still on web-2", got)
	}
	if !strings.Contains(tuitest.Screen(back), tuitest.PodCrash) {
		t.Errorf("rows are still filtered:\n%s", tuitest.Screen(back))
	}

	open, _ := tuitest.Drive(t, back, tuitest.Press("/"), tuitest.Type("w"))
	if _, _, used := open.Back(); used {
		t.Error("Back with the filter open is the filter's Esc, not the view's")
	}
}

// TestRestoreScrollsTheRowIntoView: a restored row below the first
// screenful is selected and on screen.
func TestRestoreScrollsTheRowIntoView(t *testing.T) {
	t.Parallel()

	objs := make([]runtime.Object, 0, 41)
	objs = append(objs, tuitest.Namespace(demo))
	for i := range 40 {
		objs = append(objs, tuitest.Pod(demo, fmt.Sprintf("pod-%02d", i), nil, tuitest.ContainerApp))
	}
	st := views.State{Selected: key("pod-30"), Cursor: 30}
	m, _ := restored(t, tuitest.NewFakeClient(t, tuitest.WithObjects(objs...)), st, asynctest.NewClock(epoch))
	if got := selected(m); got != st.Selected {
		t.Fatalf("cursor on %q, want %q", got, st.Selected)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "pod-30") || strings.Contains(screen, "pod-00") {
		t.Errorf("pod-30 is not scrolled into view:\n%s", screen)
	}
}
