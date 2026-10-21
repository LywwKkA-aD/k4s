package tuitest

import (
	"path/filepath"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// These tests change the environment, so none runs in parallel. Each
// starts with t.Setenv so prepare's own Setenv calls are undone.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("KUBECONFIG", "")
}

func TestPrepareNamesTheFirstContextCurrent(t *testing.T) {
	isolateEnv(t)

	cleanup, err := prepare([]string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	got, err := k8s.ListContexts("")
	if err != nil || len(got) != 2 {
		t.Fatalf("contexts = %+v, %v; want alpha and beta", got, err)
	}
	for _, c := range got {
		if c.Current != (c.Name == "alpha") {
			t.Errorf("context %s: Current = %v", c.Name, c.Current)
		}
	}
	cleanup()
	if left, err := k8s.ListContexts(""); err == nil && len(left) > 0 {
		t.Errorf("cleanup left the kubeconfig behind: %+v", left)
	}
}

func TestPrepareFailsWithoutATempDir(t *testing.T) {
	isolateEnv(t)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	if cleanup, err := prepare(nil); err == nil {
		cleanup()
		t.Fatal("prepare succeeded without a temp directory")
	}
}
