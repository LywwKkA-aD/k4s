package forwards

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

func TestRegisterPersistsAndLists(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	m := newTestManager(t, deps{statePath: path})

	changed := m.Changes()
	mustRegister(t, m, testForward("f1", "pod", 18080))
	select {
	case <-changed:
	default:
		t.Error("Register did not signal a change")
	}

	updated := testForward("f1", "pod", 18081)
	mustRegister(t, m, updated)

	a := statusOf(t, m, "f1")
	if a.Status != StatusStopped || a.Forward != updated {
		t.Errorf("listed %+v, want stopped %+v", a, updated)
	}
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(onDisk.Forwards) != 1 || onDisk.Forwards[0] != updated {
		t.Errorf("on disk = %+v, want only %+v", onDisk.Forwards, updated)
	}
}

func TestRegisterRejectsInvalidForward(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	m := newTestManager(t, deps{statePath: path})

	bad := testForward("f1", "statefulset", 18080)
	assertErrContains(t, m.Register(bad), "unknown kind")
	if len(m.List()) != 0 {
		t.Errorf("List = %+v, want empty", m.List())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file stat = %v, want it not to exist", err)
	}
}

func TestListFiltersByContextAndSorts(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, deps{})
	other := testForward("other", "pod", 1)
	other.Context = "prod"
	shared := testForward("shared", "pod", 2)
	shared.Context = ""
	b := testForward("b", "service", 3)
	a := testForward("a", "pod", 4)
	a.Namespace = "aaa"
	c := testForward("c", "pod", 5)
	mustRegister(t, m, other, shared, b, a, c)

	list := m.List()
	got := make([]string, 0, len(list))
	for _, x := range list {
		got = append(got, x.Forward.ID)
	}
	want := []string{"a", "shared", "c", "b"}
	if len(got) != len(want) {
		t.Fatalf("List IDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List IDs = %v, want %v", got, want)
		}
	}
	if len(m.State().Forwards) != 5 {
		t.Errorf("State keeps %d forwards, want all 5", len(m.State().Forwards))
	}
}

func TestStopNormalisesErrorAndRejectsUnknown(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, deps{})
	mustRegister(t, m, testForward("f1", "pod", 18080))

	assertErrContains(t, m.Start(context.Background(), "f1"), "no kubernetes client")
	if err := m.Stop("f1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	a := statusOf(t, m, "f1")
	if a.Status != StatusStopped || a.Err != nil {
		t.Errorf("after Stop = %v (err: %v), want stopped without error", a.Status, a.Err)
	}
	assertErrContains(t, m.Stop("missing"), `forward "missing" not found`)
}

func TestStopAll(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	fake.resolveDeployment = func(context.Context, string) (string, error) { return "", errors.New("no ready pod") }
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m,
		testForward("run1", "pod", 18081),
		testForward("run2", "pod", 18082),
		testForward("broken", "deployment", 18083),
	)

	ids := []string{"run1", "run2", "broken"}
	sessions := make([]*fakeSession, 0, 2)
	for _, id := range ids[:2] {
		done := startAsync(context.Background(), m, id)
		s := fake.next(t)
		s.ready()
		if err := awaitErr(t, done); err != nil {
			t.Fatalf("Start(%s): %v", id, err)
		}
		sessions = append(sessions, s)
	}
	assertErrContains(t, m.Start(context.Background(), "broken"), "no ready pod")

	changed := m.Changes()
	if err := m.StopAll(); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	select {
	case <-changed:
	default:
		t.Error("StopAll did not signal a change")
	}
	for _, s := range sessions {
		if !s.isStopped() {
			t.Errorf("session for %q still open after StopAll", s.Pod)
		}
	}
	for _, id := range ids {
		if a := statusOf(t, m, id); a.Status != StatusStopped || a.Err != nil {
			t.Errorf("%s after StopAll = %v (err: %v), want stopped without error", id, a.Status, a.Err)
		}
	}
}

func TestRemoveStopsAndForgets(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	path := filepath.Join(t.TempDir(), "portforwards.json")
	m := newTestManager(t, deps{tunnel: fake, statePath: path})
	keep := testForward("keep", "pod", 18081)
	mustRegister(t, m, keep, testForward("gone", "pod", 18082))

	done := startAsync(context.Background(), m, "gone")
	sess := fake.next(t)
	sess.ready()
	if err := awaitErr(t, done); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Remove("gone"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !sess.isStopped() {
		t.Error("Remove left the session open")
	}
	if _, ok := find(m.List(), "gone"); ok {
		t.Error("removed forward is still listed")
	}
	if _, ok := m.State().FindByID("gone"); ok {
		t.Error("removed forward is still in State")
	}
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(onDisk.Forwards) != 1 || onDisk.Forwards[0] != keep {
		t.Errorf("on disk = %+v, want only %+v", onDisk.Forwards, keep)
	}
	if err := m.Remove("missing"); err != nil {
		t.Errorf("Remove(missing) = %v, want nil", err)
	}
}

// lockableDir returns a temporary directory that setWritable can make
// read-only, skipping where permission bits do not stop the process.
func lockableDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this user")
	}
	dir := t.TempDir()
	t.Cleanup(func() { setWritable(t, dir, true) })
	return dir
}

func setWritable(t *testing.T, dir string, writable bool) {
	t.Helper()
	mode := os.FileMode(0o500)
	if writable {
		mode = 0o700
	}
	// G302 is about file modes; a directory needs its search bit.
	if err := os.Chmod(dir, mode); err != nil { //nolint:gosec // see above
		t.Fatalf("chmod %s: %v", dir, err)
	}
}

func TestRegisterWhenSaveFails(t *testing.T) {
	t.Parallel()
	dir := lockableDir(t)
	setWritable(t, dir, false)
	path := filepath.Join(dir, "portforwards.json")
	m := newTestManager(t, deps{statePath: path})

	assertErrContains(t, m.Register(testForward("f1", "pod", 18080)), "save state")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file stat = %v, want it not to exist", err)
	}
}

func TestRemoveWhenSaveFails(t *testing.T) {
	t.Parallel()
	dir := lockableDir(t)
	path := filepath.Join(dir, "portforwards.json")
	m := newTestManager(t, deps{statePath: path})
	f := testForward("f1", "pod", 18080)
	mustRegister(t, m, f)
	setWritable(t, dir, false)

	assertErrContains(t, m.Remove("f1"), "save state")
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(onDisk.Forwards) != 1 || onDisk.Forwards[0] != f {
		t.Errorf("on disk = %+v, want the forward to survive a failed save", onDisk.Forwards)
	}
}

func TestNewManagerFailsOnCorruptState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := newManager(deps{statePath: path}); err == nil {
		t.Fatal("newManager accepted a corrupt state file")
	}
}

// TestNewManagerFromClient cannot run in parallel: it points
// XDG_STATE_HOME at a temporary directory.
func TestNewManagerFromClient(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	seed := State{Forwards: []Forward{testForward("dev1", "pod", 18080), {
		ID: "prod1", Context: "prod", Namespace: testNamespace, Kind: "pod", Name: "db", LocalPort: 15432, RemotePort: 5432,
	}}}
	if err := seed.Save(); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	client := &k8s.Client{Context: testContext, RestConfig: &rest.Config{Host: "https://127.0.0.1:6443"}}
	m, err := NewManager(client)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	list := m.List()
	if len(list) != 1 || list[0].Forward.ID != "dev1" || list[0].Status != StatusStopped {
		t.Errorf("List = %+v, want only dev1, stopped", list)
	}

	noClient, err := NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager(nil): %v", err)
	}
	if got := len(noClient.List()); got != 2 {
		t.Errorf("without a client List has %d forwards, want all 2", got)
	}
	assertErrContains(t, noClient.Start(context.Background(), "dev1"), "no kubernetes client")
}

func TestNewIDIsHexAndUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 100 {
		id := NewID()
		if len(id) != 16 {
			t.Fatalf("NewID() = %q, want 16 characters", id)
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("NewID() = %q is not hex: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("NewID() repeated %q", id)
		}
		seen[id] = true
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   Status
		want string
	}{
		{StatusStopped, "stopped"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusError, "error"},
		{Status(42), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", int(tt.in), got, tt.want)
		}
	}
}
