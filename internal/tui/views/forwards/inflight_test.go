package forwards_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
)

// press delivers one key straight to the view and returns the Cmd it
// started, without running it: the operation stays in flight.
func press(t *testing.T, m forwardsview.Model, k string) (forwardsview.Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tuitest.Press(k))
	got, ok := next.(forwardsview.Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return got, cmd
}

func deliver(t *testing.T, m forwardsview.Model, msg tea.Msg) forwardsview.Model {
	t.Helper()
	next, _ := m.Update(msg)
	got, ok := next.(forwardsview.Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return got
}

func TestRepeatedKeyOnARowWhoseOperationRunsIsIgnored(t *testing.T) {
	for _, k := range []string{"d", "s", "r"} {
		t.Run(k, func(t *testing.T) {
			mgr := newManager(t, forward("pod", "alpha", 18081, 80))
			m := mounted(t, mgr)

			m, first := press(t, m, k)
			if first == nil {
				t.Fatal("the first key started nothing")
			}
			m, again := press(t, m, k)
			if again != nil {
				t.Error("a repeat while the operation runs started a second one")
			}
			m, other := press(t, m, "enter")
			if other != nil {
				t.Error("another key on the busy row started an operation")
			}

			// The result frees the row.
			m = deliver(t, m, first())
			if k == "d" {
				return
			}
			if _, next := press(t, m, k); next == nil {
				t.Error("the key is still ignored after the operation ended")
			}
		})
	}
}

func TestDeleteTwiceDeletesExactlyTheRowsShown(t *testing.T) {
	mgr := newManager(t,
		forward("pod", "alpha", 18081, 80),
		forward("pod", "beta", 18082, 80),
		forward("pod", "gamma", 18083, 80),
	)
	m := mounted(t, mgr)

	// alpha goes first; a second d on alpha is a no-op, so only the next d
	// on the row now under the cursor deletes beta.
	m, first := press(t, m, "d")
	m, dup := press(t, m, "d")
	if dup != nil {
		t.Fatal("d on the row being deleted started a second delete")
	}
	msg := first()
	m = deliver(t, m, msg)
	m, second := press(t, m, "d")
	if second == nil {
		t.Fatal("d on the next row started nothing")
	}
	out := second()
	m = deliver(t, m, out)

	got := names(mgr)
	if len(got) != 1 || got[0] != "gamma" {
		t.Errorf("forwards left = %v, want only gamma", got)
	}
	if screen := tuitest.Screen(m); strings.Contains(screen, "the forward is gone") {
		t.Errorf("a delete reported a gone forward:\n%s", screen)
	}
}

// A restart refused by the gate must not stop the forward first: the Manager
// would mutate (and notify) and the user is left with a stopped forward.
func TestRestartDuringTheGateLeavesTheForwardAlone(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	port := freePort(t)
	old := managerWith(t, []forwards.Option{forwards.WithTunneler(newFakeTunneler(t, true))},
		forward("pod", "alpha", port, 80))
	if err := old.Start(t.Context(), old.List()[0].Forward.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	next := managerWith(t, []forwards.Option{forwards.After(old.BeginStopAll())})
	m := mounted(t, next)
	changes := next.Changes()

	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	requireStatus(t, out, status.Error, "restart pod/alpha: "+forwards.ErrStopping.Error())
	select {
	case <-changes:
		t.Error("a refused restart touched the forward")
	default:
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "alpha") {
		t.Errorf("the row is gone:\n%s", screen)
	}
}
