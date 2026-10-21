package deployments_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

// restored builds a fresh view the way the root brings one back on Esc:
// New, Restore, then Init. It returns the view, every message Restore and
// Init produced, and how many lists ran before Init.
func restored(t *testing.T, st views.State, rec *listRecorder) (deployments.Model, []tea.Msg, int) {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...), rec.option())
	clk := newClock()
	fresh, _ := tuitest.Drive(t, deployments.New(c, shop, deployments.WithClock(clk)), tuitest.Resize(width, height))
	v, cmd := fresh.Restore(st)
	m, ok := v.(deployments.Model)
	if !ok {
		t.Fatalf("Restore returned %T", v)
	}
	fromRestore := tuitest.Collect(t, cmd)
	listsBeforeInit := len(rec.versions())
	m, out := tuitest.Init(t, m)
	return m, slices.Concat(fromRestore, out), listsBeforeInit
}

var (
	_ views.Stateful    = deployments.Model{}
	_ views.Restorer    = deployments.Model{}
	_ views.BackHandler = deployments.Model{}
)

func TestStateNamesTheCursorRow(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...)), shop)
	m, _ = tuitest.Drive(t, m, on("pay", tuitest.Press("down"), tuitest.Press("down"), tuitest.Press("w"))...)
	want := views.State{Filter: "pay", Selected: "shop/pay-worker", Cursor: 2, Paused: true}
	if got := m.State(); got != want {
		t.Errorf("State = %+v, want %+v", got, want)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("m"))
	if got := m.State().Filter; got != "" {
		t.Errorf("a filter being typed was kept: %q", got)
	}
}

func TestRestoreBringsTheListBack(t *testing.T) {
	t.Parallel()

	var rec listRecorder
	st := views.State{Filter: "pay", Selected: "shop/pay-worker", Cursor: 2, Paused: true}
	m, out, listsBeforeInit := restored(t, st, &rec)
	if listsBeforeInit != 0 {
		t.Errorf("Restore started %d lists of its own; Init lists", listsBeforeInit)
	}
	if got := rec.versions(); !slices.Equal(got, []string{"0"}) {
		t.Errorf("restore lists = %q, want one from the watch cache", got)
	}
	if got := m.State(); got != st {
		t.Errorf("State after restore = %+v, want %+v", got, st)
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "/ pay") || strings.Contains(screen, " web ") {
		t.Errorf("filter not reapplied:\n%s", screen)
	}
	if got := watchHint(m); got != "paused" {
		t.Errorf("hint = %q, want paused", got)
	}
	for _, msg := range out {
		if s, ok := msg.(views.StatusMsg); ok {
			t.Errorf("restore of a present row said %q", s.Text)
		}
	}
}

func TestRestoreHoldsTheRowUntilTheListArrives(t *testing.T) {
	t.Parallel()

	st := views.State{Selected: "shop/payments", Cursor: 3}
	m, _ := deployments.New(nil, shop).Restore(st)
	stateful, ok := m.(views.Stateful)
	if !ok {
		t.Fatalf("%T is not Stateful", m)
	}
	if got := stateful.State(); got.Selected != st.Selected || got.Cursor != st.Cursor {
		t.Errorf("State before the list = %+v, want the pending row %+v", got, st)
	}
}

func TestRestoreOfAGoneRow(t *testing.T) {
	t.Parallel()

	var rec listRecorder
	m, out, _ := restored(t, views.State{Selected: "shop/pay-legacy", Cursor: 9}, &rec)
	want := views.StatusMsg{Level: status.Info, Text: "deployment pay-legacy is gone"}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v among %v", want, out)
	}
	if got := m.State(); got.Selected != "shop/web" || got.Cursor != 4 {
		t.Errorf("cursor = %+v, want the last row (shop/web, 4)", got)
	}
}

func TestBackClearsACommittedFilterFirst(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...)), shop)
	m, _ = tuitest.Drive(t, m, on("pay", tuitest.Press("down"))...)

	next, cmd, used := m.Back()
	if !used || cmd != nil {
		t.Fatalf("Back with a filter: used=%v cmd=%v", used, cmd != nil)
	}
	cleared, ok := next.(deployments.Model)
	if !ok {
		t.Fatalf("Back returned %T", next)
	}
	if screen := tuitest.Screen(cleared); strings.Contains(screen, "/ pay") || !strings.Contains(screen, " web ") {
		t.Errorf("filter not cleared:\n%s", screen)
	}
	if got := cleared.State().Selected; got != "shop/pay-gateway" {
		t.Errorf("cursor moved off its row: %q", got)
	}
	if _, _, used := cleared.Back(); used {
		t.Error("Back without a filter must let Esc go back")
	}

	open, _ := tuitest.Drive(t, m, tuitest.Press("/"))
	if _, _, used := open.Back(); used {
		t.Error("Back with the filter open must leave Esc to the filter")
	}
}
