package describe_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// The test binary doubles as kubectl: fakeKubectl's runner config points
// LookPath at it and sets fakeEnv, and TestMain then plays the mode named
// by fakeModeEnv instead of running the tests.
const (
	fakeEnv     = "K4S_FAKE_KUBECTL"
	fakeModeEnv = "K4S_FAKE_MODE"
	// fakeArgvEnv names the file the fake writes its describe argv to, as
	// JSON; fakeVersionsEnv the file it appends a line to per version call.
	fakeArgvEnv     = "K4S_FAKE_ARGV"
	fakeVersionsEnv = "K4S_FAKE_VERSIONS"

	fakeVersion  = "v1.33.9"
	fakeDescribe = "Name:         from-kubectl\nNamespace:    demo\n"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) == "1" {
		os.Exit(playKubectl(os.Getenv(fakeModeEnv), os.Args[1:]))
	}
	os.Exit(m.Run())
}

// playKubectl answers `kubectl version --client -o json` and `kubectl
// describe ...` the way mode says.
func playKubectl(mode string, args []string) int {
	if slices.Contains(args, "version") {
		return playVersion(mode)
	}
	if err := recordArgv(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 98
	}
	switch mode {
	case "ok", "noversion", "garbledversion", "blankversion", "longversion":
		fmt.Print(fakeDescribe)
		return 0
	case "warn":
		fmt.Fprintln(os.Stderr, "Warning: v1 Endpoints is deprecated in v1.33+")
		fmt.Print(fakeDescribe)
		return 0
	case "escape":
		fmt.Print("Name:  \x1b]52;c;cm0gLXJmIH4=\x07evil\x1b[2J\nNext: line\r\n")
		return 0
	case "fail":
		fmt.Fprint(os.Stderr, "Error from server (Forbidden): pods \"web-1\" is forbidden\nmore\n")
		return 1
	case "sleep":
		time.Sleep(30 * time.Second)
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown fake mode %q\n", mode)
	return 99
}

func playVersion(mode string) int {
	if f := os.Getenv(fakeVersionsEnv); f != "" {
		out, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G703: the path is the test's own temp file
		if err == nil {
			_, _ = out.WriteString("version\n")
			_ = out.Close()
		}
	}
	switch mode {
	case "noversion":
		fmt.Fprintln(os.Stderr, "error: unknown flag: --client")
		return 1
	case "garbledversion":
		fmt.Println("Client Version: v1.33.9")
		return 0
	case "blankversion":
		fmt.Println(`{"clientVersion":{"gitVersion":"\u001b[31m"}}`)
		return 0
	case "longversion":
		fmt.Printf(`{"clientVersion":{"gitVersion":%q}}`+"\n", "v1.33.9+"+strings.Repeat("k3s", 20))
		return 0
	}
	fmt.Printf(`{"clientVersion":{"major":"1","minor":"33","gitVersion":%q},"kustomizeVersion":"v5.6.0"}`+"\n", fakeVersion)
	return 0
}

func recordArgv(args []string) error {
	f := os.Getenv(fakeArgvEnv)
	if f == "" {
		return nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode argv: %w", err)
	}
	if err := os.WriteFile(f, b, 0o600); err != nil { //nolint:gosec // G703: the path is the test's own temp file
		return fmt.Errorf("write argv: %w", err)
	}
	return nil
}

// fake is a kubectl played by this test binary.
type fake struct {
	cfg      kubectl.RunnerConfig
	argv     string
	versions string
}

// fakeKubectl returns a runner config whose kubectl is this binary
// playing mode; found false makes kubectl missing from PATH.
func fakeKubectl(t *testing.T, mode string, found bool) fake {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("find test binary: %v", err)
	}
	dir := t.TempDir()
	f := fake{argv: dir + "/argv.json", versions: dir + "/versions"}
	// A -cover test binary warns on stderr at exit without GOCOVERDIR.
	env := []string{
		fakeEnv + "=1", fakeModeEnv + "=" + mode,
		fakeArgvEnv + "=" + f.argv, fakeVersionsEnv + "=" + f.versions,
		"GOCOVERDIR=" + dir,
	}
	f.cfg = kubectl.RunnerConfig{
		LookPath: func(file string) (string, error) {
			if file == "kubectl" && found {
				return self, nil
			}
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		},
		Environ: func() []string { return env },
	}
	return f
}

// ranArgv is the argv the fake's describe got, nil before it ran.
func (f fake) ranArgv(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(f.argv)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	var args []string
	if err := json.Unmarshal(b, &args); err != nil {
		t.Fatalf("decode argv %q: %v", b, err)
	}
	return args
}

// versionCalls counts the fake's `kubectl version` runs.
func (f fake) versionCalls(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(f.versions)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	return strings.Count(string(b), "\n")
}
