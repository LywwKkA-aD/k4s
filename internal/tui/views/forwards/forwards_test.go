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

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
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
	return managerWith(t, nil, fwds...)
}

// managerWith builds a Manager on the state directory the test already
// set, with opts.
func managerWith(t *testing.T, opts []forwards.Option, fwds ...forwards.Forward) *forwards.Manager {
	t.Helper()
	mgr, err := forwards.NewManager(nil, opts...)
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
		ID: forwards.NewID(), Context: tuitest.FakeContext, Namespace: tuitest.NamespaceDemo,
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

// statusOf returns the one status message in out.
func statusOf(t *testing.T, out []tea.Msg) views.StatusMsg {
	t.Helper()
	all := tuitest.All[views.StatusMsg](out)
	if len(all) != 1 {
		t.Fatalf("want one status message, got:\n%s", tuitest.Describe(out))
	}
	return all[0]
}

func requireStatus(t *testing.T, out []tea.Msg, level status.Level, text string) {
	t.Helper()
	got := statusOf(t, out)
	if got.Level != level || got.Text != text {
		t.Errorf("status = %v %q, want %v %q", got.Level, got.Text, level, text)
	}
}

func names(mgr *forwards.Manager) []string {
	list := mgr.List()
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Forward.Name)
	}
	return out
}

func TestWithoutManager(t *testing.T) {
	m := forwardsview.New(nil)
	if m.Init() != nil {
		t.Error("Init without a manager should not watch anything")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Resize(width, height))
	for _, k := range []string{"enter", "s", "r", "d"} {
		_, out := tuitest.Drive(t, m, tuitest.Press(k))
		requireStatus(t, out, status.Warn, "port-forward subsystem unavailable")
	}
	if _, out := tuitest.Drive(t, m, tuitest.Press("ctrl+r")); len(out) != 0 {
		t.Errorf("ctrl+r without a manager emitted:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "port-forward subsystem unavailable") || !m.Command().IsZero() {
		t.Errorf("screen=%q kubectl=%q", tuitest.Screen(m), m.Command().String())
	}
}

func TestEmptyList(t *testing.T) {
	m := mounted(t, newManager(t))
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "no port-forwards yet") || !strings.Contains(screen, "press 'f'") {
		t.Errorf("empty list screen:\n%s", screen)
	}
	if got := m.Command(); !got.IsZero() {
		t.Errorf("Command = %q, want none without a row", got.String())
	}
	for _, k := range []string{"enter", "s", "r", "d"} {
		_, out := tuitest.Drive(t, m, tuitest.Press(k))
		requireStatus(t, out, status.Warn, "no port-forward selected")
	}
}

func TestRowsAndCommand(t *testing.T) {
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
	if got := m.Command(); got.String() != "kubectl port-forward -n demo pod/web-1 18081:8080" || got.Approx {
		t.Errorf("first row = %q (approx %v)", got.String(), got.Approx)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	if got := m.Command(); got.String() != "kubectl port-forward -n demo service/web 18080:80" || got.Approx {
		t.Errorf("second row = %q (approx %v)", got.String(), got.Approx)
	}
}

func TestCommandSkipsNamesKubectlWouldReject(t *testing.T) {
	bad := forward("pod", "Not_A_Pod", 18081, 80)
	m := mounted(t, newManager(t, bad))
	if got := m.Command(); !got.IsZero() {
		t.Errorf("Command = %q, want none for a name kubectl rejects", got.String())
	}
}

// The cursor, the keys and the footer read the rows on screen, never a
// fresh list: a forward registered behind the view's back sorts first here,
// and must not take the place of the row the user is on.
func TestKeysActOnTheRowShown(t *testing.T) {
	mgr := newManager(t, forward("pod", "bravo", 18082, 80), forward("pod", "charlie", 18083, 80))
	m := mounted(t, mgr)
	if err := mgr.Register(forward("pod", "alpha", 18081, 80)); err != nil {
		t.Fatal(err)
	}
	if got := m.Command().String(); got != "kubectl port-forward -n demo pod/bravo 18082:80" {
		t.Errorf("Command = %q, want the row on screen", got)
	}

	m, out := tuitest.Drive(t, m, tuitest.Press("d"))
	requireStatus(t, out, status.OK, "removed pod/bravo (18082 → 80)")
	if got := strings.Join(names(mgr), ","); got != "alpha,charlie" {
		t.Errorf("forwards left = %s, want alpha,charlie", got)
	}
	screen := tuitest.Screen(m)
	if strings.Contains(screen, "bravo") || !strings.Contains(screen, "alpha") {
		t.Errorf("rows were not rebuilt right after the remove:\n%s", screen)
	}
}

func TestDeleteTwiceDeletesTheRowsShown(t *testing.T) {
	mgr := newManager(t,
		forward("pod", "alpha", 18081, 80),
		forward("pod", "bravo", 18082, 80),
		forward("pod", "charlie", 18083, 80),
	)
	m := mounted(t, mgr)

	removed := make([]string, 0, 2)
	for range 2 {
		cmd := m.Command().String()
		var out []tea.Msg
		m, out = tuitest.Drive(t, m, tuitest.Press("d"))
		text := statusOf(t, out).Text
		removed = append(removed, text)
		if !strings.Contains(cmd, strings.Fields(text)[1]) {
			t.Errorf("d on %q reported %q", cmd, text)
		}
	}
	if got := strings.Join(names(mgr), ","); got != "charlie" {
		t.Errorf("forwards left = %s after %q, want charlie", got, removed)
	}
	screen := tuitest.Screen(m)
	if strings.Contains(screen, "alpha") || strings.Contains(screen, "bravo") || !strings.Contains(screen, "charlie") {
		t.Errorf("screen after two deletes:\n%s", screen)
	}
}

func TestCtrlRRefreshes(t *testing.T) {
	mgr := newManager(t, forward("pod", "bravo", 18082, 80))
	m := mounted(t, mgr)
	if err := mgr.Register(forward("pod", "alpha", 18081, 80)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tuitest.Screen(m), "alpha") {
		t.Fatal("the change reached the view without a watcher")
	}
	m, out := tuitest.Drive(t, m, tuitest.Press("ctrl+r"))
	if len(out) != 0 {
		t.Errorf("ctrl+r emitted:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "alpha") {
		t.Errorf("ctrl+r did not refresh:\n%s", tuitest.Screen(m))
	}
	// The cursor stays on the forward it was on.
	if got := m.Command().String(); got != "kubectl port-forward -n demo pod/bravo 18082:80" {
		t.Errorf("Command after the refresh = %q", got)
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
	if _, out := tuitest.Init(t, m); len(out) != 0 {
		t.Errorf("a closed view emitted:\n%s", tuitest.Describe(out))
	}
	tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
}

func TestStaticContract(t *testing.T) {
	m := forwardsview.New(nil)
	want := []string{"enter", "s", "r", "d", "ctrl+r"}
	if got := tuitest.HelpKeys(m); m.Title() != "forwards" || m.CapturesKeys() || strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Title=%q help=%v", m.Title(), got)
	}
}
