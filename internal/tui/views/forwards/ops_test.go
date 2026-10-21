package forwards_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestRowKeysWithoutACluster(t *testing.T) {
	mgr := newManager(t, forward("service", tuitest.ServiceWeb, 18080, 80))
	m := mounted(t, mgr)

	// Without a cluster client, start fails at once; the row and the
	// status line say why.
	m, out := tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.Error, "start service/web: no kubernetes client")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "error") || !strings.Contains(screen, "no kubernetes client") {
		t.Errorf("after enter:\n%s", screen)
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("s"))
	requireStatus(t, out, status.OK, "stopped service/web (18080 → 80)")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "stopped") || strings.Contains(screen, "no kubernetes client") {
		t.Errorf("s stops and clears the error:\n%s", screen)
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("r"))
	requireStatus(t, out, status.Error, "restart service/web: no kubernetes client")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "no kubernetes client") {
		t.Errorf("r restarts:\n%s", screen)
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("d"))
	requireStatus(t, out, status.OK, "removed service/web (18080 → 80)")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "no port-forwards yet") {
		t.Errorf("d removes the forward:\n%s", screen)
	}
	if n := len(mgr.State().Forwards); n != 0 {
		t.Errorf("d left %d forwards in the saved state", n)
	}
}

func TestStartAndRestartThroughATunnel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	port := freePort(t)
	mgr := managerWith(t, []forwards.Option{forwards.WithTunneler(newFakeTunneler(t, false))},
		forward("deployment", "api", port, 8080))
	t.Cleanup(func() { _ = mgr.StopAll() })
	m := mounted(t, mgr)
	want := fmt.Sprintf("forwarding localhost:%d → deployment/api:8080", port)

	m, out := tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.OK, want)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "running") {
		t.Errorf("after enter:\n%s", screen)
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("r"))
	requireStatus(t, out, status.OK, want)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "running") {
		t.Errorf("after r:\n%s", screen)
	}
}

func TestStopOfAForwardGoneBehindTheView(t *testing.T) {
	gone := forward("pod", "alpha", 18081, 80)
	mgr := newManager(t, gone)
	m := mounted(t, mgr)
	if err := mgr.Remove(gone.ID); err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"s", "r"} {
		_, out := tuitest.Drive(t, m, tuitest.Press(k))
		op := map[string]string{"s": "stop", "r": "restart"}[k]
		requireStatus(t, out, status.Warn, op+" pod/alpha: the forward is gone")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("s"))
	if screen := tuitest.Screen(m); strings.Contains(screen, "alpha") {
		t.Errorf("the gone row is still shown after the op:\n%s", screen)
	}
}

func TestRemoveReportsAFailedSave(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this user")
	}
	mgr := newManager(t, forward("pod", "alpha", 18081, 80))
	dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "k4s")
	lock(t, dir)
	m := mounted(t, mgr)

	m, out := tuitest.Drive(t, m, tuitest.Press("d"))
	got := statusOf(t, out)
	if got.Level != status.Error || !strings.HasPrefix(got.Text, "remove pod/alpha: save state: ") {
		t.Errorf("status = %v %q, want the save error", got.Level, got.Text)
	}
	// The forward is still saved, so it is still listed.
	if screen := tuitest.Screen(m); !strings.Contains(screen, "alpha") || !strings.Contains(screen, "stopped") {
		t.Errorf("a forward whose removal was not saved left the list:\n%s", screen)
	}
}

// lock makes dir read-only for the rest of the test.
func lock(t *testing.T, dir string) {
	t.Helper()
	// G302 is about file modes; a directory needs its search bit.
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // see above
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // see above
			t.Errorf("chmod: %v", err)
		}
	})
}

// After a context switch the new Manager waits for the old one's sessions:
// a start in the meantime is refused, with the reason on the status line.
func TestStartWhileThePreviousContextStops(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	port := freePort(t)
	old := managerWith(t, []forwards.Option{forwards.WithTunneler(newFakeTunneler(t, true))},
		forward("pod", "alpha", port, 80))
	if err := old.Start(t.Context(), old.List()[0].Forward.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stop := old.BeginStopAll()
	next := managerWith(t, []forwards.Option{forwards.After(stop)})

	m := mounted(t, next)
	m, out := tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.Error, "start pod/alpha: "+forwards.ErrStopping.Error())
	if screen := tuitest.Screen(m); !strings.Contains(screen, "stopped") {
		t.Errorf("a refused start changed the row:\n%s", screen)
	}
}
