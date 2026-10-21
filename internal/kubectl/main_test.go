package kubectl_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a fake kubectl: the runner tests point
// LookPath at it and set fakeEnv, and TestMain then plays the mode named by
// fakeModeEnv instead of running tests.
const (
	fakeEnv     = "K4S_FAKE_KUBECTL"
	fakeModeEnv = "K4S_FAKE_MODE"
)

// fakeReport is what the "echo" mode prints: the argv it got, every
// KUBECONFIG entry in its environment, and whether stdin was the null device.
type fakeReport struct {
	Args        []string `json:"args"`
	Kubeconfigs []string `json:"kubeconfigs"`
	StdinNull   bool     `json:"stdinNull"`
}

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) == "1" {
		os.Exit(fakeKubectl(os.Getenv(fakeModeEnv), os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeKubectl(mode string, args []string) int {
	switch mode {
	case "echo":
		return fakeEcho(args)
	case "fail":
		fmt.Fprint(os.Stderr, "\x1b[31mError from server (NotFound)\x1b[0m: pods \"web\" not found\nsecond line\n")
		return 3
	case "quiet-fail":
		return 2
	case "flood":
		_, _ = os.Stdout.WriteString(strings.Repeat("o", 9<<20))
		_, _ = os.Stderr.WriteString(strings.Repeat("e", 100<<10))
		return 0
	case "sleep":
		time.Sleep(30 * time.Second)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown fake mode %q\n", mode)
		return 99
	}
}

func fakeEcho(args []string) int {
	var kubeconfigs []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "KUBECONFIG=") {
			kubeconfigs = append(kubeconfigs, strings.TrimPrefix(kv, "KUBECONFIG="))
		}
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	report := fakeReport{Args: args, Kubeconfigs: kubeconfigs, StdinNull: stdinIsNull()}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return 98
	}
	return 0
}

func stdinIsNull() bool {
	in, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	null, err := os.Stat(os.DevNull)
	if err != nil {
		return false
	}
	return os.SameFile(in, null)
}
