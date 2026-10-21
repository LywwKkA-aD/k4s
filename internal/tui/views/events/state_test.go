package events_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/events"
)

// restored builds a fresh view, restores st and mounts it, the way the
// root brings a list back on Esc.
func restored(t *testing.T, m events.Model, st views.State) (events.Model, []tea.Msg) {
	t.Helper()
	next, cmd := m.Restore(st)
	r, ok := next.(events.Model)
	if !ok {
		t.Fatalf("Restore returned %T", next)
	}
	r, out := tuitest.Mount(t, r, width, height)
	return r, append(tuitest.Collect(t, cmd), out...)
}

// TestStateRoundTrip: filter, warnings only, the row under the cursor and
// a paused watch come back as they were left.
func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	c := client(t)
	m, _ := mounted(t, c, "")
	m, _ = tuitest.Drive(t, m, tuitest.Press("W"))
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("node")...)
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	st := m.State()
	want := views.State{Filter: "node", Selected: views.RowKey("default", evNotReady), Paused: true, Mode: "warnings"}
	if st != want {
		t.Fatalf("State = %+v, want %+v", st, want)
	}

	fresh, _ := clocked(c, "")
	back, _ := restored(t, fresh, st)
	if got := back.State(); got != want {
		t.Errorf("restored State = %+v, want %+v", got, want)
	}
	if got := back.Command().String(); got != "kubectl events -A --types=Warning" {
		t.Errorf("restored Command = %q", got)
	}
	if got := helpLabel(t, back, "w"); got != "paused" {
		t.Errorf("restored w label = %q", got)
	}
}

// TestRestoredRowMovedKeepsTheCursorOnIt: a newer event arrives before
// the restored list does; the cursor still lands on the row it was on.
func TestRestoredRowMovedKeepsTheCursorOnIt(t *testing.T) {
	t.Parallel()

	c := client(t)
	m, _ := mounted(t, c, demo)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	st := m.State()
	if st.Selected != views.RowKey(demo, evPulled) {
		t.Fatalf("selected %q", st.Selected)
	}
	newer := ev(demo, "web-2.started", "Normal", "Started", "started", pod(demo, "web-2"), 1, 0, 0)
	if _, err := c.Clientset.CoreV1().Events(demo).Create(context.Background(), newer, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	fresh, _ := clocked(c, demo)
	back, _ := restored(t, fresh, st)
	if ref, _ := back.Selected(); ref.Name != evPulled {
		t.Errorf("cursor on %q, want %q", ref.Name, evPulled)
	}
}

// TestRestoredRowGoneSaysSo: an event that expired while the user was
// away leaves the cursor at its index and a status.
func TestRestoredRowGoneSaysSo(t *testing.T) {
	t.Parallel()

	c := client(t)
	m, _ := mounted(t, c, demo)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	st := m.State()
	if err := c.Clientset.CoreV1().Events(demo).Delete(context.Background(), evPulled, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete event: %v", err)
	}
	fresh, _ := clocked(c, demo)
	back, out := restored(t, fresh, st)
	requireStatus(t, out, status.Info, "event "+evPulled+" is gone")
	if ref, _ := back.Selected(); ref.Name != evScaled {
		t.Errorf("cursor on %q, want the row now at index 1, %q", ref.Name, evScaled)
	}
}

// TestRestoredFilterShowsAtOnce: the first frame of a restored view is
// already filtered, before its rows arrive.
func TestRestoredFilterShowsAtOnce(t *testing.T) {
	t.Parallel()

	fresh, _ := clocked(client(t), demo)
	next, _ := fresh.Restore(views.State{Filter: "pulled"})
	if screen := tuitest.Screen(next); !strings.Contains(screen, "pulled") {
		t.Errorf("first frame lacks the filter:\n%s", screen)
	}
	if st := next.(events.Model).State(); st.Filter != "pulled" {
		t.Errorf("State before the list = %+v", st)
	}
}
