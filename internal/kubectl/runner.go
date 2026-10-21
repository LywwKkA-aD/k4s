package kubectl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds a non-interactive run when RunnerConfig sets none.
	DefaultTimeout = 10 * time.Second
	// MaxStdout and MaxStderr cap what Run keeps of each stream; the rest
	// is drained and dropped so kubectl never blocks on a full pipe.
	MaxStdout = 8 << 20
	MaxStderr = 64 << 10

	// waitDelay bounds how long Run waits for the pipes after kubectl
	// exits or is killed: an exec credential plugin it started can hold
	// them open.
	waitDelay = 2 * time.Second
)

// ErrNotFound is returned when neither kubectl nor k3s is on PATH.
var ErrNotFound = errors.New("kubectl not found on PATH")

// RunnerConfig configures a Runner. The zero value runs the kubectl on PATH
// with the parent environment and DefaultTimeout.
type RunnerConfig struct {
	// Kubeconfig lists the files k4s loaded; KUBECONFIG is set to them so
	// kubectl reads the same config. Empty inherits the parent's KUBECONFIG.
	Kubeconfig []string
	// Timeout bounds Run; zero means DefaultTimeout.
	Timeout time.Duration
	// LookPath resolves a program name; nil means exec.LookPath.
	LookPath func(file string) (string, error)
	// Environ returns the base environment; nil means os.Environ.
	Environ func() []string
}

// Runner runs Cmds as argv, never through a shell. It falls back to the
// k3s multicall binary (`k3s kubectl`) when kubectl is not on PATH.
type Runner struct {
	kubeconfig []string
	timeout    time.Duration
	lookPath   func(string) (string, error)
	environ    func() []string
}

// NewRunner returns a Runner for cfg. Lookup happens on every call, so a
// kubectl installed while k4s runs is picked up.
func NewRunner(cfg RunnerConfig) *Runner {
	r := &Runner{
		kubeconfig: cleanPaths(cfg.Kubeconfig),
		timeout:    cfg.Timeout,
		lookPath:   cfg.LookPath,
		environ:    cfg.Environ,
	}
	if r.timeout <= 0 {
		r.timeout = DefaultTimeout
	}
	if r.lookPath == nil {
		r.lookPath = exec.LookPath
	}
	if r.environ == nil {
		r.environ = os.Environ
	}
	return r
}

// Program returns the words that start kubectl: its path, or the k3s path
// followed by "kubectl".
func (r *Runner) Program() ([]string, error) {
	path, err := r.lookPath(Name)
	if err == nil {
		return []string{path}, nil
	}
	if k3s, k3sErr := r.lookPath("k3s"); k3sErr == nil {
		return []string{k3s, Name}, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("find kubectl: %w", err)
}

// Command returns c as an *exec.Cmd for an interactive run such as
// `kubectl exec -it`: no timeout and no stdio, which tea.ExecProcess wires
// to the terminal.
func (r *Runner) Command(c Cmd) (*exec.Cmd, error) {
	argv, err := r.argv(c)
	if err != nil {
		return nil, err
	}
	// gosec G204: argv is a validated Cmd run without a shell.
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec
	cmd.Env = r.env(c)
	return cmd, nil
}

// Result is what a finished non-interactive run produced.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	// Truncated is set when either stream went past its cap.
	Truncated bool
}

// Run runs c with stdin at the null device, within the Runner's timeout
// and ctx. A non-zero exit returns the Result and an *ExitError; a timeout
// or cancel returns an error wrapping the context's error.
func (r *Runner) Run(ctx context.Context, c Cmd) (Result, error) {
	argv, err := r.argv(c)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	stdout := &cappedBuffer{limit: MaxStdout}
	stderr := &cappedBuffer{limit: MaxStderr}
	// gosec G204: argv is a validated Cmd run without a shell.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec
	cmd.Env = r.env(c)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay
	runErr := cmd.Run()

	res := Result{
		Stdout:    stdout.buf.Bytes(),
		Stderr:    stderr.buf.Bytes(),
		Truncated: stdout.truncated || stderr.truncated,
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	return res, runError(ctx, c, runErr, res)
}

func runError(ctx context.Context, c Cmd, runErr error, res Result) error {
	if runErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("run kubectl %s: %w", c.Args[0], ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return &ExitError{Code: res.ExitCode, Stderr: firstLine(res.Stderr)}
	}
	return fmt.Errorf("run kubectl %s: %w", c.Args[0], runErr)
}

// ExitError reports a kubectl that ran and exited non-zero.
type ExitError struct {
	Code int
	// Stderr is the first non-empty line kubectl wrote, made printable.
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("kubectl exited with status %d", e.Code)
	}
	return fmt.Sprintf("kubectl exited with status %d: %s", e.Code, e.Stderr)
}

func (r *Runner) argv(c Cmd) ([]string, error) {
	if err := checkArgs(c); err != nil {
		return nil, err
	}
	prog, err := r.Program()
	if err != nil {
		return nil, err
	}
	return append(prog, c.Argv()[1:]...), nil
}

// env returns the base environment with KUBECONFIG pointed at the files
// k4s loaded, then c's own assignments on top.
func (r *Runner) env(c Cmd) []string {
	env := r.environ()
	if len(r.kubeconfig) > 0 {
		env = setEnv(env, EnvKubeconfig+"="+strings.Join(r.kubeconfig, string(filepath.ListSeparator)))
	}
	for _, kv := range c.Env() {
		env = setEnv(env, kv)
	}
	return env
}

// setEnv returns a copy of env with kv's name set to kv's value. Windows
// names are case-insensitive, so "Kubeconfig=" goes too there.
func setEnv(env []string, kv string) []string {
	key, _, _ := strings.Cut(kv, "=")
	fold := runtime.GOOS == "windows"
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		if name == key || (fold && strings.EqualFold(name, key)) {
			continue
		}
		out = append(out, e)
	}
	return append(out, kv)
}
