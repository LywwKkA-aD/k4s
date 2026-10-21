package namespaces_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
)

var (
	_ views.Stateful    = namespaces.Model{}
	_ views.Restorer    = namespaces.Model{}
	_ views.BackHandler = namespaces.Model{}
)

const allRow = "<all>"

func TestStateNamesFilterRowAndWatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []tea.Msg
		want views.State
	}{
		{name: "fresh", want: views.State{Selected: allRow}},
		{
			name: "committed filter and a row",
			keys: slices.Concat(tuitest.FilterBy("de"), tuitest.Keys("down", "down")),
			want: views.State{Filter: "de", Selected: tuitest.NamespaceDemo, Cursor: 2},
		},
		{name: "paused", keys: tuitest.Keys("down", "w"), want: views.State{Selected: tuitest.NamespaceDefault, Cursor: 1, Paused: true}},
		{
			name: "a query still being typed is not kept",
			keys: []tea.Msg{tuitest.Press("/"), tuitest.Type("kube")},
			want: views.State{Selected: allRow},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := clocked(t, tuitest.NewFakeClient(t))
			m, _ = tuitest.Drive(t, m, tc.keys...)
			if got := m.State(); got != tc.want {
				t.Errorf("State = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// restored builds a view, restores st into it and mounts it the way the
// root does: Restore first, then a resize and Init.
func restored(t *testing.T, st views.State) (namespaces.Model, []tea.Msg, *lists) {
	t.Helper()
	c, l := recorded(t)
	m, _ := fresh(c)
	v, cmd := m.Restore(st)
	if out := tuitest.Collect(t, cmd); len(out) != 0 || len(l.seen()) != 0 {
		t.Fatalf("Restore fetched or emitted: lists %q, out:\n%s", l.seen(), tuitest.Describe(out))
	}
	m, ok := v.(namespaces.Model)
	if !ok {
		t.Fatalf("Restore returned %T", v)
	}
	if got := m.State(); got != st {
		t.Errorf("State before the first list = %+v, want the restored %+v", got, st)
	}
	m, out := tuitest.Mount(t, m, width, height)
	return m, out, l
}

func selectedOn(t *testing.T, m namespaces.Model) string {
	t.Helper()
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	sel, ok := tuitest.Find[views.NamespaceSelectedMsg](out)
	if !ok {
		t.Fatalf("enter selected nothing:\n%s", tuitest.Describe(out))
	}
	return sel.Namespace
}

func TestRestoreLandsOnTheSavedRow(t *testing.T) {
	t.Parallel()

	m, out, l := restored(t, views.State{Filter: "de", Selected: tuitest.NamespaceDemo, Cursor: 1})
	if got := l.seen(); !slices.Equal(got, []string{"0"}) {
		t.Errorf("lists after Restore and Init = %q, want one from the cache", got)
	}
	if len(tuitest.All[views.StatusMsg](out)) != 0 {
		t.Errorf("a row that is still there reported:\n%s", tuitest.Describe(out))
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "/ de") || strings.Contains(screen, tuitest.NamespaceSystem) {
		t.Errorf("filter not restored:\n%s", screen)
	}
	if got := selectedOn(t, m); got != tuitest.NamespaceDemo {
		t.Errorf("cursor on %q, want %q", got, tuitest.NamespaceDemo)
	}
}

func TestRestoreOfAGoneRow(t *testing.T) {
	t.Parallel()

	m, out, _ := restored(t, views.State{Selected: "payments", Cursor: 9})
	want := views.StatusMsg{Level: status.Info, Text: "namespace payments is gone"}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	if got := selectedOn(t, m); got != tuitest.NamespaceSystem {
		t.Errorf("cursor on %q, want the last row %q", got, tuitest.NamespaceSystem)
	}
}

func TestRestorePaused(t *testing.T) {
	t.Parallel()

	m, clk := fresh(tuitest.NewFakeClient(t))
	v, _ := m.Restore(views.State{Selected: allRow, Paused: true})
	m, _ = tuitest.Mount(t, v.(namespaces.Model), width, height)
	if got := watchHint(t, m); got != "paused" || clk.Pending() != 0 {
		t.Errorf("restored paused: hint %q, %d ticks armed", got, clk.Pending())
	}
	if !m.State().Paused {
		t.Error("State lost Paused")
	}
}

func TestBackClearsOnlyACommittedFilter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		keys     []tea.Msg
		wantUsed bool
	}{
		{name: "committed filter", keys: tuitest.FilterBy("dem"), wantUsed: true},
		{name: "no filter", wantUsed: false},
		{name: "open filter", keys: []tea.Msg{tuitest.Press("/"), tuitest.Type("dem")}, wantUsed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := clocked(t, tuitest.NewFakeClient(t))
			m, _ = tuitest.Drive(t, m, tc.keys...)
			next, cmd, used := m.Back()
			if used != tc.wantUsed || cmd != nil {
				t.Fatalf("Back used=%v cmd=%v, want used=%v and no Cmd", used, cmd != nil, tc.wantUsed)
			}
			if !used {
				return
			}
			screen := tuitest.Screen(next)
			if strings.Contains(screen, "/ dem") || !strings.Contains(screen, tuitest.NamespaceSystem) {
				t.Errorf("Back left the filter in place:\n%s", screen)
			}
			if st := next.(namespaces.Model).State(); st.Filter != "" {
				t.Errorf("State after Back = %+v", st)
			}
		})
	}
}
