package forwards_test

// Every test here owns a state directory through t.Setenv, so none of them
// runs in parallel. That also keeps the goroutine baselines honest: the
// count is process-wide.

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
)

const (
	width  = 140
	height = 20
	// viewPkg prefixes every frame of the view under test; a parked
	// goroutine with such a frame is its change watcher.
	viewPkg = "internal/tui/views/forwards."
)

func newManager(t *testing.T, fwds ...forwards.Forward) *forwards.Manager {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, err := forwards.NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for _, f := range fwds {
		if err := mgr.Register(f); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	return mgr
}

func forward(kind, name string, local, remote uint16) forwards.Forward {
	return forwards.Forward{
		ID: forwards.NewID(), Namespace: tuitest.NamespaceDemo,
		Kind: kind, Name: name, LocalPort: local, RemotePort: remote,
	}
}

func mounted(t *testing.T, mgr *forwards.Manager) forwardsview.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, forwardsview.New(mgr), width, height)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// noWatchers waits until watchers of earlier tests' views have exited, so
// stack checks below see this test's watcher only.
func noWatchers(t *testing.T) {
	t.Helper()
	tuitest.Eventually(t, 2*time.Second, func() bool { return tuitest.Parked(viewPkg) == 0 },
		"a change watcher from an earlier test is still running")
}

// rebuilt resizes m, which re-reads the manager's list.
func rebuilt(t *testing.T, m forwardsview.Model) string {
	t.Helper()
	m, _ = tuitest.Drive(t, m, tuitest.Resize(width, height))
	return tuitest.Screen(m)
}

func TestWithoutManager(t *testing.T) {
	m := forwardsview.New(nil)
	if m.Init() != nil {
		t.Error("Init without a manager should not watch anything")
	}
	m, out := tuitest.Drive(t, m, tuitest.Resize(width, height), tuitest.Enter(), tuitest.Press("d"))
	if len(out) != 0 {
		t.Errorf("keys without a manager emitted:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "port-forward subsystem unavailable") || m.KubectlEquivalent() != "" {
		t.Errorf("screen=%q kubectl=%q", tuitest.Screen(m), m.KubectlEquivalent())
	}
}

func TestEmptyList(t *testing.T) {
	m := mounted(t, newManager(t))
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "no port-forwards yet") || !strings.Contains(screen, "press 'f'") {
		t.Errorf("empty list screen:\n%s", screen)
	}
	if got := m.KubectlEquivalent(); got != "kubectl port-forward …" {
		t.Errorf("KubectlEquivalent = %q", got)
	}
	if _, out := tuitest.Drive(t, m, tuitest.Keys("enter", "s", "r", "d")...); len(out) != 0 {
		t.Errorf("keys on an empty list emitted:\n%s", tuitest.Describe(out))
	}
}

func TestRowsAndKubectlEquivalent(t *testing.T) {
	mgr := newManager(t,
		forward("service", tuitest.ServiceWeb, 18080, 80),
		forward("pod", tuitest.PodWeb1, 18081, 8080),
	)
	m := mounted(t, mgr)
	screen := tuitest.Screen(m)
	for _, w := range []string{"STATUS", "LOCAL → REMOTE", "stopped", "18080 → 80", "18081 → 8080", tuitest.PodWeb1} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	// Rows sort by namespace, kind, name: the pod comes before the service.
	if got := m.KubectlEquivalent(); got != "kubectl port-forward -n demo pod/web-1 18081:8080" {
		t.Errorf("first row = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	if got := m.KubectlEquivalent(); got != "kubectl port-forward -n demo service/web 18080:80" {
		t.Errorf("second row = %q", got)
	}
}

func TestRowKeys(t *testing.T) {
	mgr := newManager(t, forward("service", tuitest.ServiceWeb, 18080, 80))
	m := mounted(t, mgr)

	// Without a cluster client, start fails at once and the row says why.
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if screen := rebuilt(t, m); !strings.Contains(screen, "error") || !strings.Contains(screen, "no kubernetes client") {
		t.Errorf("after enter:\n%s", screen)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("s"))
	if screen := rebuilt(t, m); !strings.Contains(screen, "stopped") || strings.Contains(screen, "no kubernetes client") {
		t.Errorf("s stops and clears the error:\n%s", screen)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if screen := rebuilt(t, m); !strings.Contains(screen, "no kubernetes client") {
		t.Errorf("r restarts:\n%s", screen)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("d"))
	if screen := rebuilt(t, m); !strings.Contains(screen, "no port-forwards yet") {
		t.Errorf("d removes the forward:\n%s", screen)
	}
	if n := len(mgr.State().Forwards); n != 0 {
		t.Errorf("d left %d forwards in the saved state", n)
	}
}

func TestManagerChangesRefreshRows(t *testing.T) {
	mgr := newManager(t)
	noWatchers(t)
	m, _ := tuitest.Drive(t, forwardsview.New(mgr), tuitest.Resize(width, height))
	t.Cleanup(func() { _ = m.Close() })

	registered := make(chan error, 1)
	go func() {
		registered <- registerOnceWatching(mgr, forward("pod", tuitest.PodWeb2, 18082, 80))
	}()
	m, _ = tuitest.InitWith(t, tuitest.Driver{CmdTimeout: time.Second}, m)
	if err := <-registered; err != nil {
		t.Fatal(err)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, tuitest.PodWeb2) {
		t.Errorf("a manager change did not refresh the rows:\n%s", screen)
	}
}

// registerOnceWatching registers f as soon as the view's change watcher is
// parked, so the change cannot slip past it. It runs off the test
// goroutine and reports instead of failing.
func registerOnceWatching(mgr *forwards.Manager, f forwards.Forward) error {
	deadline := time.Now().Add(2 * time.Second)
	for tuitest.Parked(viewPkg) == 0 {
		if time.Now().After(deadline) {
			return errors.New("the change watcher never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return mgr.Register(f)
}

func TestCloseReleasesTheWatcher(t *testing.T) {
	mgr := newManager(t)
	noWatchers(t)
	base := tuitest.SettledGoroutines()

	m := forwardsview.New(mgr)
	m, _ = tuitest.Init(t, m)
	tuitest.Eventually(t, 2*time.Second, func() bool {
		return runtime.NumGoroutine() > base && tuitest.Parked(viewPkg) == 1
	}, "the change watcher is not parked")

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
	if tuitest.Parked(viewPkg) != 0 {
		t.Errorf("watcher survived Close:\n%s", tuitest.Stacks(viewPkg))
	}
}

func TestClosedViewStartsNoWatcher(t *testing.T) {
	mgr := newManager(t)
	noWatchers(t)
	base := tuitest.SettledGoroutines()

	m := forwardsview.New(mgr)
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, out := tuitest.Init(t, m)
	if len(out) != 1 {
		t.Errorf("a closed view should only refresh once:\n%s", tuitest.Describe(out))
	}
	tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
}

func TestStaticContract(t *testing.T) {
	m := forwardsview.New(nil)
	if m.Title() != "forwards" || m.CapturesKeys() || len(m.Help()) != 4 {
		t.Errorf("Title=%q help=%d", m.Title(), len(m.Help()))
	}
}
