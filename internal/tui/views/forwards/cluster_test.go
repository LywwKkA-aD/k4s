package forwards_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
)

const (
	devContext = "default"
	devServer  = "https://127.0.0.1:6443"
)

// seedLegacy writes the state file an earlier k4s left: forwards saved
// under a context name, without the server they were saved for.
func seedLegacy(t *testing.T, body string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", dir)
	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// clusterManager is a Manager for the stock k3s context "default" at
// 127.0.0.1:6443, opening its tunnels through a fake.
func clusterManager(t *testing.T) *forwards.Manager {
	t.Helper()
	client := &k8s.Client{Context: devContext, RestConfig: &rest.Config{Host: devServer}}
	mgr, err := forwards.NewManager(client, forwards.WithTunneler(newFakeTunneler(t, false)))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopAll() })
	return mgr
}

// A corrupt state file is set aside on load; the first forwards view says
// where it went, once.
func TestACorruptStateFileIsReportedOnce(t *testing.T) {
	seedLegacy(t, `{"forwards": [{"id": "x", "context": "default", "namespace": "demo", "kind": "pod", "name": "../etc", "local_port": 1, "remote_port": 1}]}`)
	mgr, err := forwards.NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m, out := tuitest.Mount(t, forwardsview.New(mgr), width, height)
	t.Cleanup(func() { _ = m.Close() })
	got := statusOf(t, out)
	if got.Level != status.Warn || !strings.HasPrefix(got.Text, "saved forwards were unreadable (") ||
		!strings.Contains(got.Text, "portforwards.json.corrupt-") {
		t.Errorf("status = %v %q, want the corrupt file and where it went", got.Level, got.Text)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "no port-forwards yet") {
		t.Errorf("the view is not usable after a corrupt file:\n%s", screen)
	}

	again, out := tuitest.Mount(t, forwardsview.New(mgr), width, height)
	t.Cleanup(func() { _ = again.Close() })
	if all := tuitest.All[views.StatusMsg](out); len(all) != 0 {
		t.Errorf("a second view repeated the notice: %+v", all)
	}
}

func TestALegacyForwardIsConfirmedByStartingIt(t *testing.T) {
	port := freePort(t)
	seedLegacy(t, fmt.Sprintf(`{"forwards": [
		{"id": "legacy", "context": %q, "namespace": "demo", "kind": "pod", "name": "alpha", "local_port": %d, "remote_port": 80},
		{"id": "elsewhere", "context": %q, "server": "https://10.0.0.9:6443", "namespace": "demo", "kind": "pod", "name": "bravo", "local_port": 18082, "remote_port": 80}
	]}`, devContext, port, devContext))
	mgr := clusterManager(t)
	m := mounted(t, mgr)

	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "alpha") || !strings.Contains(screen, "unknown cluster, confirm") {
		t.Errorf("the legacy forward is not offered for confirmation:\n%s", screen)
	}
	if strings.Contains(screen, "bravo") {
		t.Errorf("a forward of another server under the same context name is listed:\n%s", screen)
	}

	m, out := tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.OK,
		fmt.Sprintf("forwarding localhost:%d → pod/alpha:80 (saved for default · 127.0.0.1:6443)", port))
	screen = tuitest.Screen(m)
	if !strings.Contains(screen, "running") || strings.Contains(screen, "unknown cluster") {
		t.Errorf("after the confirmation:\n%s", screen)
	}
	if a := mgr.List(); len(a) != 1 || a[0].Unbound || a[0].Forward.Server != devServer {
		t.Errorf("List = %+v, want the forward bound to %s", a, devServer)
	}
}
