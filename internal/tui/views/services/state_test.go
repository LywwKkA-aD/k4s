package services_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

var (
	_ views.Stateful    = services.Model{}
	_ views.Restorer    = services.Model{}
	_ views.BackHandler = services.Model{}
)

// payState is the services view with "pay" committed and the cursor on
// the third match.
var payState = views.State{Filter: "pay", Selected: shop + "/pay-web", Cursor: 2}

func TestStateIsWhatTheUserLeft(t *testing.T) {
	t.Parallel()

	m := mounted(t, payClient(t), shop)
	filtered, _ := tuitest.Drive(t, m, on("pay", tuitest.Press("down"), tuitest.Press("down"))...)
	paused, _ := tuitest.Drive(t, filtered, tuitest.Press("w"))
	reopened, _ := tuitest.Drive(t, filtered, tuitest.Press("/"))
	cases := []struct {
		name string
		m    services.Model
		want views.State
	}{
		{name: "fresh", m: m, want: views.State{Selected: shop + "/api"}},
		{name: "filtered, third match", m: filtered, want: payState},
		{name: "paused", m: paused, want: views.State{Filter: "pay", Selected: shop + "/pay-web", Cursor: 2, Paused: true}},
		{name: "an open filter is not kept", m: reopened, want: views.State{Selected: shop + "/pay-web", Cursor: 2}},
		{name: "no client", m: services.New(nil, ""), want: views.State{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.m.State(); got != tc.want {
				t.Fatalf("State = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// restored builds a fresh view in namespace, restores st and starts it
// the way the root does: Restore first, then Init.
func restored(t *testing.T, c *k8s.Client, namespace string, st views.State) (services.Model, []tea.Msg) {
	t.Helper()
	m, _ := clocked(c, namespace)
	v, cmd := m.Restore(st)
	r, ok := v.(services.Model)
	if !ok {
		t.Fatalf("Restore returned %T", v)
	}
	fromRestore := tuitest.Collect(t, cmd)
	r, out := tuitest.Mount(t, r, width, height)
	return r, slices.Concat(fromRestore, out)
}

func TestRestoreBringsTheListBack(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, out := restored(t, payClient(t, log.reactor()), shop, payState)
	if got := m.State(); got != payState {
		t.Fatalf("State after restore = %+v, want %+v", got, payState)
	}
	if got := describes(t, m); got != payState.Selected {
		t.Fatalf("enter describes %q, want %q", got, payState.Selected)
	}
	if !strings.Contains(tuitest.Screen(m), "/ pay") {
		t.Fatalf("the filter bar is gone:\n%s", tuitest.Screen(m))
	}
	if strings.Contains(tuitest.Screen(m), "8005") {
		t.Fatalf("the restored filter lets web through:\n%s", tuitest.Screen(m))
	}
	if got := log.seen(); !slices.Equal(got, []string{"0"}) {
		t.Fatalf("restore listed %q, want one list from the watch cache", got)
	}
	if len(statuses(out)) != 0 {
		t.Fatalf("a row that is still there set a status:\n%s", tuitest.Describe(out))
	}
}

// TestRestoredFilterAppliesAtOnce: the first frame after Restore, before
// any list arrives, already shows the filter.
func TestRestoredFilterAppliesAtOnce(t *testing.T) {
	t.Parallel()

	m, _ := clocked(payClient(t), shop)
	v, _ := m.Restore(payState)
	v, _ = v.Update(tuitest.Resize(width, height))
	if got := v.(services.Model).State(); got != payState {
		t.Fatalf("State before the first list = %+v, want %+v", got, payState)
	}
	if v.CapturesKeys() {
		t.Fatal("a restored filter is committed, not open")
	}
}

func TestRestoreOfAGoneRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		cursor    int
		want      string
		status    string
	}{
		{name: "lands on the same index", namespace: shop, cursor: 2, want: shop + "/payments", status: "service pay-web is gone"},
		{name: "clamps to the last row", namespace: shop, cursor: 9, want: shop + "/payments", status: "service pay-web is gone"},
		{
			name: "all namespaces name the namespace", cursor: 2, want: shop + "/payments",
			status: "service " + shop + "/pay-web is gone",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := payClient(t)
			deleteService(t, c, shop, "pay-web")
			st := payState
			st.Cursor = tc.cursor
			m, out := restored(t, c, tc.namespace, st)
			requireStatus(t, out, status.Info, tc.status)
			if got := describes(t, m); got != tc.want {
				t.Fatalf("enter describes %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRestoreKeepsWatchPaused(t *testing.T) {
	t.Parallel()

	m, clk := clocked(payClient(t), shop)
	v, cmd := m.Restore(views.State{Paused: true})
	if msgs := tuitest.Collect(t, cmd); len(msgs) != 0 {
		t.Fatalf("Restore started work of its own:\n%s", tuitest.Describe(msgs))
	}
	r, _ := tuitest.Mount(t, v.(services.Model), width, height)
	if clk.Pending() != 0 {
		t.Fatalf("a paused view armed %d ticks", clk.Pending())
	}
	if got := watchLabel(t, r); got != "paused" {
		t.Fatalf("w label = %q, want paused", got)
	}
	if !r.State().Paused {
		t.Fatal("State lost Paused")
	}
}

// TestRestoreScrollsTheRowIntoView: a row far down a long list is on
// screen once restored.
func TestRestoreScrollsTheRowIntoView(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(manyServices(40)...))
	m, _ := clocked(c, shop)
	v, _ := m.Restore(views.State{Selected: shop + "/svc-35", Cursor: 35})
	r, _ := tuitest.Mount(t, v.(services.Model), width, 12)
	if !strings.Contains(tuitest.Screen(r), "svc-35") {
		t.Fatalf("svc-35 is off screen:\n%s", tuitest.Screen(r))
	}
	if got := describes(t, r); got != shop+"/svc-35" {
		t.Fatalf("enter describes %q", got)
	}
}

func TestBackClearsACommittedFilter(t *testing.T) {
	t.Parallel()

	m := mounted(t, payClient(t), shop)
	m, _ = tuitest.Drive(t, m, on("pay", tuitest.Press("down"), tuitest.Press("down"))...)

	v, cmd, used := m.Back()
	if !used || cmd != nil {
		t.Fatalf("Back with a committed filter: used=%v cmd=%v, want used and no Cmd", used, cmd)
	}
	cleared := v.(services.Model)
	if got := cleared.State(); got.Filter != "" || got.Selected != shop+"/pay-web" {
		t.Fatalf("State after Back = %+v, want no filter and the cursor still on pay-web", got)
	}
	if strings.Contains(tuitest.Screen(cleared), "/ pay") {
		t.Fatalf("the filter bar is still up:\n%s", tuitest.Screen(cleared))
	}
	if _, _, used := cleared.Back(); used {
		t.Fatal("Back without a filter must leave Esc to the root")
	}
}

// manyServices is n services in shop named svc-00, svc-01, ...
func manyServices(n int) []runtime.Object {
	objs := make([]runtime.Object, 0, n)
	for i := range n {
		objs = append(objs, tuitest.Service(shop, fmt.Sprintf("svc-%02d", i), 80, map[string]string{"app": "x"}))
	}
	return objs
}
