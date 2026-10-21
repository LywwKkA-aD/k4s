package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain keeps the root model the black-box tests build away from the
// developer's machine: XDG_STATE_HOME so no saved port-forward opens the
// restore popup, KUBECONFIG so the footer commands do not depend on the
// developer's kubeconfig.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "k4s-services-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state")); err != nil {
		panic(err)
	}
	kubeconfig, err := tuitest.WriteKubeconfig(tmp, tuitest.FakeContext, tuitest.FakeContext)
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		panic(err)
	}
	return m.Run()
}
