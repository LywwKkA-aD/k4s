package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// Contexts in the kubeconfig TestMain writes. The first is current.
const (
	testContextAlpha = "alpha"
	testContextBeta  = "beta"
)

// TestMain isolates every test in this package (internal and external
// test files share the binary) from the user's real $HOME:
//
//   - XDG_STATE_HOME points at a throwaway directory. Without it, New()
//     loads ~/.local/state/k4s/portforwards.json from the developer's
//     machine and may open the restore popup unbidden, which would break
//     tests that assume a clean start.
//   - KUBECONFIG points at a generated file with the contexts alpha and
//     beta, so the contexts view and the context switch never read the
//     developer's clusters.
//
// Tests that need their own state directory use t.Setenv and do not run in
// parallel.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "k4s-tui-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state")); err != nil {
		panic(err)
	}
	kubeconfig, err := tuitest.WriteKubeconfig(tmp, testContextAlpha, testContextAlpha, testContextBeta)
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		panic(err)
	}
	return m.Run()
}
