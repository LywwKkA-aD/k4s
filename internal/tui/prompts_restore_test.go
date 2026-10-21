package tui

// These tests give the port-forward state its own directory with t.Setenv
// and do not run in parallel.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const restoreServer = "https://127.0.0.1:6443"

// stockK3s is a client for a stock k3s kubeconfig: context "default" at
// 127.0.0.1:6443, like every other k3s server's.
func stockK3s() *k8s.Client {
	return &k8s.Client{Context: "default", RestConfig: &rest.Config{Host: restoreServer}}
}

// seedRestoreState saves one forward per relation to stockK3s: saved for
// it, saved before k4s recorded servers, saved for another server under
// the same context name, and saved for another context.
func seedRestoreState(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"forwards": [
		{"id": "here", "context": "default", "server": "` + restoreServer + `", "namespace": "demo", "kind": "pod", "name": "here", "local_port": 19001, "remote_port": 80},
		{"id": "legacy", "context": "default", "namespace": "demo", "kind": "pod", "name": "legacy", "local_port": 19002, "remote_port": 80},
		{"id": "other-host", "context": "default", "server": "https://10.0.0.9:6443", "namespace": "demo", "kind": "pod", "name": "other-host", "local_port": 19003, "remote_port": 80},
		{"id": "prod", "context": "prod", "server": "https://10.0.0.7:6443", "namespace": "demo", "kind": "pod", "name": "prod", "local_port": 19004, "remote_port": 80}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreOffersOnlyThisClustersForwards(t *testing.T) {
	seedRestoreState(t)
	client := stockK3s()
	mgr, err := forwards.NewManager(client)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if got := forwardsForContext(mgr.State().Forwards, client.Context); got != 2 {
		t.Errorf("forwardsForContext = %d, want 2: the other server's forward is not this cluster's", got)
	}
	o := restoreOverlay(mgr, client.Context, time.Now)
	if o == nil {
		t.Fatal("no restore question for two saved forwards")
	}
	screen := o.View(80)
	for _, want := range []string{
		"Found 2 saved forwards from a previous session, for:",
		"\n  default · 127.0.0.1:6443\n",
		"\n  default · unknown cluster, confirm\n",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("restore question lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "10.0.0.") {
		t.Errorf("restore question offers another cluster:\n%s", screen)
	}
}

func TestRestoreAsksNothingWithoutSavedForwards(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, err := forwards.NewManager(stockK3s())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if o := restoreOverlay(mgr, "default", time.Now); o != nil {
		t.Errorf("restore question with nothing saved:\n%s", o.View(80))
	}
	if o := restoreOverlay(nil, "default", time.Now); o != nil {
		t.Error("restore question without a Manager")
	}
}

// Reviving is the confirmation the unknown-cluster forward waits for: it is
// saved for this cluster and started. The other cluster's forwards are not
// touched.
func TestReviveBindsTheLegacyForwardAndStartsThisClusters(t *testing.T) {
	seedRestoreState(t)
	tun := &hungTunneler{}
	t.Cleanup(tun.release)
	client := stockK3s()
	mgr, err := forwards.NewManager(client, forwards.WithTunneler(tun))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	Model{forwardMgr: mgr, client: client}.reviveAll()

	deadline := time.Now().Add(5 * time.Second)
	for !allRunningAndBound(mgr.List()) {
		if time.Now().After(deadline) {
			t.Fatalf("after the revival List = %+v, want here and legacy running and bound", mgr.List())
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, f := range mgr.State().Forwards {
		if f.ID == "legacy" && f.Server != restoreServer {
			t.Errorf("legacy is saved for %q, want %q", f.Server, restoreServer)
		}
	}
	if got := len(mgr.State().Forwards); got != 3 {
		t.Errorf("State has %d forwards, want 3: here, legacy and prod", got)
	}
}

func allRunningAndBound(list []forwards.Active) bool {
	if len(list) != 2 {
		return false
	}
	for _, a := range list {
		if a.Status != forwards.StatusRunning || a.Unbound {
			return false
		}
	}
	return true
}
