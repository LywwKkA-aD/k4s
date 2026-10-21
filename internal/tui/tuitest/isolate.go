package tuitest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Isolate runs m with the environment the root model reads taken off the
// developer's machine, and returns its exit code:
//
//   - XDG_STATE_HOME points at an empty directory, so no saved port-forward
//     opens the restore popup unbidden.
//   - KUBECONFIG points at a generated file (see WriteKubeconfig) with one
//     context per name, the first of them current; none means FakeContext
//     alone. Nothing reads the real clusters.
//
// Use it from TestMain: os.Exit(tuitest.Isolate(m)). Tests that need
// another file use t.Setenv and do not run in parallel.
func Isolate(m *testing.M, contexts ...string) int {
	cleanup, err := prepare(contexts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tuitest.Isolate:", err)
		return 1
	}
	defer cleanup()
	return m.Run()
}

// prepare points the environment at a fresh directory and returns what
// removes it.
func prepare(contexts []string) (func(), error) {
	if len(contexts) == 0 {
		contexts = []string{FakeContext}
	}
	dir, err := os.MkdirTemp("", "k4s-test-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state")); err != nil {
		cleanup()
		return nil, fmt.Errorf("set XDG_STATE_HOME: %w", err)
	}
	kubeconfig, err := WriteKubeconfig(dir, contexts[0], contexts...)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("write kubeconfig: %w", err)
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		cleanup()
		return nil, fmt.Errorf("set KUBECONFIG: %w", err)
	}
	return cleanup, nil
}
