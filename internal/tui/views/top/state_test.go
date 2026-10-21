package top_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
)

var (
	_ views.Stateful = top.Model{}
	_ views.Restorer = top.Model{}
)

// Row keys in the demo seed: pods across namespaces, busiest CPU first.
const (
	keyMulti = tuitest.NamespaceDemo + "/" + tuitest.PodMulti
	keyWeb1  = tuitest.NamespaceDemo + "/" + tuitest.PodWeb1
	keyWeb2  = tuitest.NamespaceDemo + "/" + tuitest.PodWeb2
)

func TestStateNamesModeSortRowAndWatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []tea.Msg
		want views.State
	}{
		{name: "fresh", want: views.State{Selected: keyMulti, Mode: "pods", Sort: "cpu"}},
		{name: "a row", keys: tuitest.Keys("down", "down"), want: views.State{Selected: keyWeb2, Cursor: 2, Mode: "pods", Sort: "cpu"}},
		{name: "nodes by memory, paused", keys: tuitest.Keys("n", "s", "w"), want: views.State{Selected: tuitest.NodeName, Paused: true, Mode: "nodes", Sort: "memory"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), ""), tc.keys...)
			if got := m.State(); got != tc.want {
				t.Errorf("State = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// restored restores st into a fresh view and mounts it the way the root
// does: Restore, then a resize and Init.
func restored(t *testing.T, st views.State) (top.Model, []tea.Msg, int) {
	t.Helper()
	m, clk := fresh(tuitest.NewFakeClient(t), "")
	v, cmd := m.Restore(st)
	if out := tuitest.Collect(t, cmd); len(out) != 0 {
		t.Fatalf("Restore fetched or emitted:\n%s", tuitest.Describe(out))
	}
	m, ok := v.(top.Model)
	if !ok {
		t.Fatalf("Restore returned %T", v)
	}
	want := st
	if want.Mode == "" {
		want.Mode = "pods"
	}
	if want.Sort == "" {
		want.Sort = "cpu"
	}
	if got := m.State(); got != want {
		t.Errorf("State before the first list = %+v, want the restored %+v", got, want)
	}
	m, out := tuitest.Mount(t, m, width, height)
	return m, out, clk.Pending()
}

func TestRestoreKeepsModeSortAndWatch(t *testing.T) {
	t.Parallel()

	st := views.State{Selected: tuitest.NodeName, Paused: true, Mode: "nodes", Sort: "memory"}
	m, out, pending := restored(t, st)
	if got := m.Command().String(); got != "kubectl top nodes --sort-by=memory" {
		t.Errorf("Command = %q", got)
	}
	if pending != 0 || hint(t, m, "w") != "paused" {
		t.Errorf("restored paused: %d ticks armed, hint %q", pending, hint(t, m, "w"))
	}
	if got := tuitest.All[views.StatusMsg](out); len(got) != 0 {
		t.Errorf("the node is still there: %+v", got)
	}
	if got := m.State(); got != st {
		t.Errorf("State = %+v, want %+v", got, st)
	}
}

func TestRestoreLandsOnTheSavedRow(t *testing.T) {
	t.Parallel()

	m, _, _ := restored(t, views.State{Selected: keyWeb1, Cursor: 3, Mode: "pods", Sort: "cpu"})
	if got := m.State(); got.Selected != keyWeb1 || got.Cursor != 1 {
		t.Errorf("State = %+v, want the cursor on %s at index 1", got, keyWeb1)
	}
}

func TestRestoreOfAGonePod(t *testing.T) {
	t.Parallel()

	m, out, _ := restored(t, views.State{Selected: "demo/api-0", Cursor: 40, Mode: "pods", Sort: "cpu"})
	want := views.StatusMsg{Level: status.Info, Text: "pod api-0 is gone"}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	if got := m.State(); got.Cursor != 4 || got.Selected != tuitest.NamespaceDemo+"/"+tuitest.PodCrash {
		t.Errorf("State = %+v, want the cursor on the last row", got)
	}
}

// TestDefaultsRestoreAsPodsByCPU: "" is a view's default in State.
func TestDefaultsRestoreAsPodsByCPU(t *testing.T) {
	t.Parallel()

	m, _, _ := restored(t, views.State{Selected: keyMulti})
	if got := m.Command().String(); got != "kubectl top pods -A" {
		t.Errorf("Command = %q", got)
	}
}

func TestRestoreOfAGoneNode(t *testing.T) {
	t.Parallel()

	_, out, _ := restored(t, views.State{Selected: "node-9", Mode: "nodes", Sort: "cpu"})
	want := views.StatusMsg{Level: status.Info, Text: "node node-9 is gone"}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
}

// TestRestoreWithoutARowKeepsTheIndex: what :ns hands over, the row
// dropped, lands quietly on the saved index.
func TestRestoreWithoutARowKeepsTheIndex(t *testing.T) {
	t.Parallel()

	m, out, _ := restored(t, views.State{Cursor: 2, Mode: "pods", Sort: "cpu"})
	if got := tuitest.All[views.StatusMsg](out); len(got) != 0 {
		t.Errorf("no saved row, yet a status: %+v", got)
	}
	if got := m.State(); got.Selected != keyWeb2 {
		t.Errorf("State = %+v, want row 2, %s", got, keyWeb2)
	}
}

func TestNoClientStartsNothing(t *testing.T) {
	t.Parallel()

	if cmd := top.New(nil, "").Init(); cmd != nil {
		t.Error("Init without a client returned a Cmd")
	}
}
