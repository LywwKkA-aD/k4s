package kubectl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// fakeConfig returns a RunnerConfig whose kubectl (and, with k3s set, only
// k3s) resolves to this test binary playing mode.
func fakeConfig(t *testing.T, mode string, k3s bool) kubectl.RunnerConfig {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("find test binary: %v", err)
	}
	// A -cover test binary warns on stderr at exit without GOCOVERDIR.
	coverDir := t.TempDir()
	return kubectl.RunnerConfig{
		LookPath: func(file string) (string, error) {
			if (file == "kubectl" && !k3s) || (file == "k3s" && k3s) {
				return self, nil
			}
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		},
		Environ: func() []string {
			return []string{fakeEnv + "=1", fakeModeEnv + "=" + mode, "KUBECONFIG=/stale", "HOME=/home/me", "GOCOVERDIR=" + coverDir}
		},
	}
}

func decodeReport(t *testing.T, out []byte) fakeReport {
	t.Helper()
	var r fakeReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("decode fake output %q: %v", out, err)
	}
	return r
}

func TestRunnerRunsExactArgv(t *testing.T) {
	t.Parallel()

	cfg := fakeConfig(t, "echo", false)
	cfg.Kubeconfig = []string{"/a/one", "/b/two"}
	cmd := kubectl.New("describe", "pod", "web", "-n", "k4s-demo").WithContext("it's prod")

	res, err := kubectl.NewRunner(cfg).Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v (stderr %q)", err, res.Stderr)
	}
	got := decodeReport(t, res.Stdout)
	if want := cmd.Argv()[1:]; !slices.Equal(got.Args, want) {
		t.Errorf("args = %q, want %q", got.Args, want)
	}
	wantCfg := strings.Join(cfg.Kubeconfig, string(filepath.ListSeparator))
	if !slices.Equal(got.Kubeconfigs, []string{wantCfg}) {
		t.Errorf("KUBECONFIG entries = %q, want only %q", got.Kubeconfigs, wantCfg)
	}
	if runtime.GOOS != "windows" && !got.StdinNull {
		t.Error("stdin is not the null device")
	}
	if res.ExitCode != 0 || res.Truncated {
		t.Errorf("result = %+v, want exit 0 and nothing truncated", res)
	}
}

func TestRunnerEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kubeconfig []string
		cmd        kubectl.Cmd
		want       []string
	}{
		{name: "no recorded files inherit the parent's", cmd: kubectl.New("version"), want: []string{"/stale"}},
		{name: "one file", kubeconfig: []string{"/a/one"}, cmd: kubectl.New("version"), want: []string{"/a/one"}},
		{
			name:       "the command's own assignment wins",
			kubeconfig: []string{"/a/one"},
			cmd:        kubectl.New("version").WithKubeconfig([]string{"/x", "/y"}),
			want:       []string{strings.Join([]string{"/x", "/y"}, string(filepath.ListSeparator))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := fakeConfig(t, "echo", false)
			cfg.Kubeconfig = tt.kubeconfig
			res, err := kubectl.NewRunner(cfg).Run(context.Background(), tt.cmd)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := decodeReport(t, res.Stdout).Kubeconfigs; !slices.Equal(got, tt.want) {
				t.Errorf("KUBECONFIG entries = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunnerFallsBackToK3s(t *testing.T) {
	t.Parallel()

	r := kubectl.NewRunner(fakeConfig(t, "echo", true))
	prog, err := r.Program()
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	if len(prog) != 2 || prog[1] != "kubectl" {
		t.Fatalf("Program() = %q, want [<k3s> kubectl]", prog)
	}
	res, err := r.Run(context.Background(), kubectl.New("get", "pods", "-n", "kube-system"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"kubectl", "get", "pods", "-n", "kube-system"}
	if got := decodeReport(t, res.Stdout).Args; !slices.Equal(got, want) {
		t.Errorf("k3s got %q, want %q", got, want)
	}
}

func TestRunnerNotFound(t *testing.T) {
	t.Parallel()

	r := kubectl.NewRunner(kubectl.RunnerConfig{
		LookPath: func(file string) (string, error) { return "", &exec.Error{Name: file, Err: exec.ErrNotFound} },
	})
	if _, err := r.Program(); !errors.Is(err, kubectl.ErrNotFound) {
		t.Errorf("Program() error = %v, want ErrNotFound", err)
	}
	if _, err := r.Run(context.Background(), kubectl.New("version")); !errors.Is(err, kubectl.ErrNotFound) {
		t.Errorf("Run() error = %v, want ErrNotFound", err)
	}
	if _, err := r.Command(kubectl.New("version")); !errors.Is(err, kubectl.ErrNotFound) {
		t.Errorf("Command() error = %v, want ErrNotFound", err)
	}
	if got := kubectl.ErrNotFound.Error(); got != "kubectl not found on PATH" {
		t.Errorf("ErrNotFound = %q", got)
	}
}

func TestRunnerRefusesKubectlFromCurrentDir(t *testing.T) {
	t.Parallel()

	r := kubectl.NewRunner(kubectl.RunnerConfig{
		LookPath: func(file string) (string, error) {
			if file == "kubectl" {
				return "./kubectl", &exec.Error{Name: file, Err: exec.ErrDot}
			}
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		},
	})
	_, err := r.Program()
	if !errors.Is(err, exec.ErrDot) {
		t.Fatalf("Program() error = %v, want it to carry exec.ErrDot", err)
	}
	if errors.Is(err, kubectl.ErrNotFound) {
		t.Errorf("a kubectl refused for safety is not reported as missing: %v", err)
	}
}

func TestRunnerExitError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode, wantMsg string
		wantCode      int
	}{
		{"fail", `kubectl exited with status 3: Error from server (NotFound): pods "web" not found`, 3},
		{"quiet-fail", "kubectl exited with status 2", 2},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			t.Parallel()
			res, err := kubectl.NewRunner(fakeConfig(t, tt.mode, false)).Run(context.Background(), kubectl.New("get", "pod", "web"))
			var exitErr *kubectl.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("err = %v, want *ExitError", err)
			}
			if exitErr.Code != tt.wantCode || res.ExitCode != tt.wantCode {
				t.Errorf("codes = %d / %d, want %d", exitErr.Code, res.ExitCode, tt.wantCode)
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("Error() = %q\nwant      %q", err.Error(), tt.wantMsg)
			}
			if tt.mode == "fail" && !strings.Contains(string(res.Stderr), "second line") {
				t.Errorf("Stderr lost the rest of the output: %q", res.Stderr)
			}
		})
	}
}

func TestRunnerStartFailure(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "kubectl")
	r := kubectl.NewRunner(kubectl.RunnerConfig{
		LookPath: func(string) (string, error) { return missing, nil },
		Environ:  func() []string { return nil },
	})
	_, err := r.Run(context.Background(), kubectl.New("version"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want it to wrap fs.ErrNotExist", err)
	}
	var exitErr *kubectl.ExitError
	if errors.As(err, &exitErr) {
		t.Errorf("a binary that never started is not an exit: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "run kubectl version: ") {
		t.Errorf("Error() = %q, want the verb in front", err.Error())
	}
}

func TestRunnerCapsOutput(t *testing.T) {
	t.Parallel()

	res, err := kubectl.NewRunner(fakeConfig(t, "flood", false)).Run(context.Background(), kubectl.New("get", "events", "-A"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Stderr) != kubectl.MaxStderr {
		t.Errorf("len(Stderr) = %d, want %d", len(res.Stderr), kubectl.MaxStderr)
	}
	if len(res.Stdout) != kubectl.MaxStdout {
		t.Errorf("len(Stdout) = %d, want %d", len(res.Stdout), kubectl.MaxStdout)
	}
	if !res.Truncated {
		t.Error("Truncated = false after a flood")
	}
}

func TestRunnerTimeout(t *testing.T) {
	t.Parallel()

	cfg := fakeConfig(t, "sleep", false)
	cfg.Timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := kubectl.NewRunner(cfg).Run(context.Background(), kubectl.New("describe", "node", "n1"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Run took %v after a 300ms timeout", elapsed)
	}
}

func TestRunnerHonoursCallerCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := kubectl.NewRunner(fakeConfig(t, "sleep", false)).Run(ctx, kubectl.New("version"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRunnerValidatesBeforeLookingUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  kubectl.Cmd
		want error
	}{
		{"zero command", kubectl.Cmd{}, kubectl.ErrNoCommand},
		{"bad namespace", kubectl.New("get", "pods", "-n", "Bad_NS"), kubectl.ErrInvalidName},
		{"bad long namespace", kubectl.New("get", "pods", "--namespace", "-A"), kubectl.ErrInvalidName},
		{"bad namespace with =", kubectl.New("get", "pods", "--namespace=a;b"), kubectl.ErrInvalidName},
		{"bad short namespace with =", kubectl.New("get", "pods", "-n=UP"), kubectl.ErrInvalidName},
		{"bad container", kubectl.New("logs", "web", "-n", "shop", "-c", "x y"), kubectl.ErrInvalidName},
		{"bad long container", kubectl.New("logs", "web", "--container=--previous"), kubectl.ErrInvalidName},
		{"dangling flag", kubectl.New("logs", "web", "-n"), kubectl.ErrInvalidName},
		{"bad attached namespace", kubectl.New("get", "pods", "-nBad_NS"), kubectl.ErrInvalidName},
		{"attached namespace that reads as a flag", kubectl.New("get", "pods", "-n-A"), kubectl.ErrInvalidName},
		{"attached namespace with =", kubectl.New("get", "pods", "-nweb=x"), kubectl.ErrInvalidName},
		{"bad attached container", kubectl.New("logs", "web", "-n", "shop", "-cX"), kubectl.ErrInvalidName},
		{"attached container that reads as a flag", kubectl.New("logs", "web", "-c--follow"), kubectl.ErrInvalidName},
		{"attached value with an s reads as -s", kubectl.New("get", "pods", "-nshop"), kubectl.ErrSessionFlag},
		{"name that sets the server", kubectl.New("describe", "pod", "--server=https://evil", "-n", "ns"), kubectl.ErrSessionFlag},
		{"name that sets the kubeconfig", kubectl.New("describe", "pod", "--kubeconfig=/tmp/evil"), kubectl.ErrSessionFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var lookups atomic.Int32
			r := kubectl.NewRunner(kubectl.RunnerConfig{LookPath: func(string) (string, error) {
				lookups.Add(1)
				return "/usr/bin/kubectl", nil
			}})
			if _, err := r.Run(context.Background(), tt.cmd); !errors.Is(err, tt.want) {
				t.Errorf("Run() error = %v, want %v", err, tt.want)
			}
			if _, err := r.Command(tt.cmd); !errors.Is(err, tt.want) {
				t.Errorf("Command() error = %v, want %v", err, tt.want)
			}
			if n := lookups.Load(); n != 0 {
				t.Errorf("LookPath ran %d times for a rejected command", n)
			}
		})
	}
}

// The flags k4s writes itself pass: -it is not read as -i plus a value,
// and a valid attached value is checked, not refused.
func TestRunnerAcceptsValidFlagForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  kubectl.Cmd
	}{
		{"split namespace and container", kubectl.New("logs", "web", "-n", "shop", "-c", "app")},
		{"attached namespace", kubectl.New("get", "pods", "-nweb")},
		{"attached container", kubectl.New("logs", "web", "-n", "shop", "-capp")},
		{"long forms", kubectl.New("logs", "web", "--namespace=shop", "--container", "app")},
		{"exec shorthands", kubectl.New("exec", "-it", "-n", "shop", "web", "--", "sh", "-c", "-nX")},
		{"other short flags", kubectl.New("logs", "-f", "-l", "app=web", "--prefix", "--tail=-1", "-n", "shop")},
		{"all namespaces and output", kubectl.New("get", "pods", "-A", "-o", "wide")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := kubectl.NewRunner(kubectl.RunnerConfig{LookPath: func(string) (string, error) {
				return "/usr/bin/kubectl", nil
			}})
			if _, err := r.Command(tt.cmd); err != nil {
				t.Errorf("Command(%q) = %v, want nil", tt.cmd.Args, err)
			}
		})
	}
}

func TestRunnerLeavesArgsAfterDoubleDashAlone(t *testing.T) {
	t.Parallel()

	cmd, err := kubectl.Exec("shop", "api-0", "app", "sh", "-c", "Not A Name; -n X")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	res, err := kubectl.NewRunner(fakeConfig(t, "echo", false)).Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := decodeReport(t, res.Stdout).Args; !slices.Equal(got, cmd.Argv()[1:]) {
		t.Errorf("args = %q, want %q", got, cmd.Argv()[1:])
	}
}

func TestRunnerCommandForInteractiveUse(t *testing.T) {
	t.Parallel()

	cfg := fakeConfig(t, "echo", false)
	cfg.Kubeconfig = []string{"/a/one"}
	cmd, err := kubectl.Exec("shop", "api-0", "", "sh")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	c, err := kubectl.NewRunner(cfg).Command(cmd)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if c.Stdin != nil || c.Stdout != nil || c.Stderr != nil {
		t.Error("Command wired stdio; tea.ExecProcess owns the terminal")
	}
	if !slices.Contains(c.Env, "KUBECONFIG=/a/one") {
		t.Errorf("Env = %q, want KUBECONFIG=/a/one", c.Env)
	}
	out, err := c.Output()
	if err != nil {
		t.Fatalf("run built command: %v", err)
	}
	if got := decodeReport(t, out).Args; !slices.Equal(got, cmd.Argv()[1:]) {
		t.Errorf("args = %q, want %q", got, cmd.Argv()[1:])
	}
}

func TestRunnerDefaultsUseTheRealPath(t *testing.T) {
	t.Parallel()

	prog, err := kubectl.NewRunner(kubectl.RunnerConfig{}).Program()
	switch {
	case err == nil && len(prog) == 0:
		t.Error("Program() returned no words and no error")
	case err != nil && !errors.Is(err, kubectl.ErrNotFound):
		t.Logf("kubectl lookup on this machine: %v", err)
	}
}
