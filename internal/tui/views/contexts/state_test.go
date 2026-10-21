package contexts_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
)

var (
	_ views.Stateful = contexts.Model{}
	_ views.Restorer = contexts.Model{}
)

func TestStateNamesTheRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []tea.Msg
		want views.State
	}{
		{name: "first row", want: views.State{Selected: alpha}},
		{name: "second row", keys: tuitest.Keys("down"), want: views.State{Selected: beta, Cursor: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, mounted(t, alpha), tc.keys...)
			if got := m.State(); got != tc.want {
				t.Errorf("State = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func selected(t *testing.T, m contexts.Model) string {
	t.Helper()
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	sel, ok := tuitest.Find[views.ContextSelectedMsg](out)
	if !ok {
		t.Fatalf("enter selected nothing:\n%s", tuitest.Describe(out))
	}
	return sel.Name
}

func TestRestore(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		st         views.State
		wantRow    string
		wantStatus []views.StatusMsg
	}{
		{name: "the saved row", st: views.State{Selected: beta}, wantRow: beta},
		{name: "no saved row keeps the index", st: views.State{Cursor: 1}, wantRow: beta},
		{
			name: "a gone row lands on the nearest one", st: views.State{Selected: "gamma", Cursor: 7}, wantRow: beta,
			wantStatus: []views.StatusMsg{{Level: status.Info, Text: "context gamma is gone"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v, cmd := contexts.New(alpha, k8s.Source{}).Restore(tc.st)
			if out := tuitest.Collect(t, cmd); len(out) != 0 {
				t.Fatalf("Restore read or emitted:\n%s", tuitest.Describe(out))
			}
			m, ok := v.(contexts.Model)
			if !ok {
				t.Fatalf("Restore returned %T", v)
			}
			if got := m.State(); got != tc.st {
				t.Errorf("State before the first read = %+v, want %+v", got, tc.st)
			}
			m, out := tuitest.Mount(t, m, 120, 20)
			if got := tuitest.All[views.StatusMsg](out); !slices.Equal(got, tc.wantStatus) {
				t.Errorf("status = %+v, want %+v", got, tc.wantStatus)
			}
			if got := selected(t, m); got != tc.wantRow {
				t.Errorf("cursor on %q, want %q", got, tc.wantRow)
			}
		})
	}
}

// TestRestoreAfterTheRead: a picker that already has its rows lands at once.
func TestRestoreAfterTheRead(t *testing.T) {
	t.Parallel()

	v, cmd := mounted(t, alpha).Restore(views.State{Selected: beta})
	if out := tuitest.Collect(t, cmd); len(out) != 0 {
		t.Errorf("Restore emitted:\n%s", tuitest.Describe(out))
	}
	if got := selected(t, v.(contexts.Model)); got != beta {
		t.Errorf("cursor on %q, want %q", got, beta)
	}
}

func colon(name string) []tea.Msg {
	return []tea.Msg{tuitest.Press(":"), tuitest.Type(name), tuitest.Enter()}
}

// rootFooter is the frame under the footer's rule: the hints or status row
// and the kubectl row.
func rootFooter(m tui.Model) string {
	lines := strings.Split(tuitest.Screen(m), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" && strings.Trim(s, "─") == "" {
			return tuitest.TrimLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

// TestEscBringsTheContextsBackOnTheSameRow goes through the root: the row
// under the cursor survives a trip to another view and back.
func TestEscBringsTheContextsBackOnTheSameRow(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(colon("ctx"), tuitest.Keys("down"))...)
	before := rootFooter(m)
	m, _ = tuitest.Drive(t, m, slices.Concat(colon("pods"), []tea.Msg{tuitest.Esc()})...)
	if after := rootFooter(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	// Step: the pick is only looked at, not handed to the root, which
	// would switch the session to it.
	_, out := tuitest.Step(t, m, tuitest.Enter())
	if !tuitest.Contains(out, views.ContextSelectedMsg{Name: beta}) {
		t.Errorf("the cursor is not back on %s:\n%s", beta, tuitest.Describe(out))
	}
}
