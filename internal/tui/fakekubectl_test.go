package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// The test binary doubles as kubectl: fakeKubectl's runner config points
// LookPath at it, and TestMain plays it when fakeKubectlEnv is set.
const (
	fakeKubectlEnv = "K4S_TUI_FAKE_KUBECTL"
	// fakeArgvEnv names the file the fake writes its describe argv to.
	fakeArgvEnv = "K4S_TUI_FAKE_ARGV"
	// fakeDescribed is what the fake prints for a describe.
	fakeDescribed = "described-by-kubectl"
)

// playFakeKubectl answers `kubectl version` and `kubectl describe` when
// the binary runs as the fake; ok is false for a normal test run.
func playFakeKubectl() (code int, ok bool) {
	if os.Getenv(fakeKubectlEnv) != "1" {
		return 0, false
	}
	args := os.Args[1:]
	if slices.Contains(args, "version") {
		fmt.Println(`{"clientVersion":{"gitVersion":"v1.33.9"}}`)
		return 0, true
	}
	b, err := json.Marshal(args)
	if err != nil {
		return 98, true
	}
	if err := os.WriteFile(os.Getenv(fakeArgvEnv), b, 0o600); err != nil { //nolint:gosec // G703: the path is the test's own temp file
		return 97, true
	}
	fmt.Println("Name:  " + fakeDescribed)
	return 0, true
}

// fakeKubectl returns a runner config whose kubectl is this binary, and
// the file its describe argv lands in.
func fakeKubectl(t *testing.T) (kubectl.RunnerConfig, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("find test binary: %v", err)
	}
	dir := t.TempDir()
	argv := dir + "/argv.json"
	// A -cover test binary warns on stderr at exit without GOCOVERDIR.
	env := []string{fakeKubectlEnv + "=1", fakeArgvEnv + "=" + argv, "GOCOVERDIR=" + dir}
	return kubectl.RunnerConfig{
		LookPath: func(file string) (string, error) {
			if file == kubectl.Name {
				return self, nil
			}
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		},
		Environ: func() []string { return env },
	}, argv
}

func ranArgv(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("kubectl did not run: %v", err)
	}
	var args []string
	if err := json.Unmarshal(b, &args); err != nil {
		t.Fatalf("decode argv %q: %v", b, err)
	}
	return args
}
