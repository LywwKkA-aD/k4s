package tui

// These tests give the port-forward state its own directory with t.Setenv
// and do not run in parallel.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const stateNotice = "forwards: saved forwards were unreadable ("

// writeCorruptState leaves a state file under dir, the XDG_STATE_HOME,
// that does not parse: the next Manager sets it aside and has a notice.
func writeCorruptState(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestContextSwitchKeepsTheStateNoticeOverTheStop: the status row holds
// one message, and the report of the old forwards' stop comes after the
// notice about the new context's corrupt state file. The report says
// both, so the notice is still on screen once the stop is done, whether
// it ended or timed out.
func TestContextSwitchKeepsTheStateNoticeOverTheStop(t *testing.T) {
	cases := []struct {
		name string
		// release lets the hung session end before the stop's deadline.
		release bool
		want    string
	}{
		{name: "a stop that ended", release: true, want: "stopped 1 forward · " + stateNotice},
		{name: "a stop that timed out", want: "forwards: stop timed out: 1 of 1 forwards still closing · " + stateNotice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_STATE_HOME", dir)
			mgr, tun := hungForwards(t, 1)
			m := New(tuitest.NewFakeClient(t), nil)
			m.forwardMgr = mgr
			// After New, whose own Manager would take the notice.
			writeCorruptState(t, dir)

			upd, cmd := m.Update(views.ContextSelectedMsg{Name: testContextBeta})
			m = upd.(Model)
			if got, ok := m.status.Current(m.now()); !ok || got.Level != status.Warn || !strings.HasPrefix(got.Text, stateNotice) {
				t.Fatalf("status after :ctx = %+v (shown %v), want the notice", got, ok)
			}
			if tc.release {
				tun.release()
			}
			msg, ok := forwardsStatus(t, cmd, 3*time.Second)
			if !ok {
				t.Fatal("the stop never reported")
			}
			upd, _ = m.Update(msg)
			m = upd.(Model)
			got, shown := m.status.Current(m.now())
			if !shown || got.Level != status.Warn || !strings.HasPrefix(got.Text, tc.want) {
				t.Errorf("status after the stop = %+v (shown %v), want a warning starting %q", got, shown, tc.want)
			}
		})
	}
}
