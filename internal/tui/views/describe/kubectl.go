package describe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

const (
	// kubectlTimeout bounds `kubectl describe`; the built-in describer
	// gets its own fetchTimeout after it.
	kubectlTimeout = 10 * time.Second
	// maxVersion bounds the version the title shows.
	maxVersion = 32
)

// Runner runs a kubectl command; *kubectl.Runner is one.
type Runner interface {
	Run(ctx context.Context, c kubectl.Cmd) (kubectl.Result, error)
}

// Kubectl is how the view runs `kubectl describe`: the session's runner
// and the flags the footer adds to every command, so what runs is what
// the footer shows. The zero Kubectl leaves the view to the built-in
// describer.
//
// A Kubectl is made once per session and shared by every describe view:
// it asks kubectl for its version once.
type Kubectl struct {
	runner   Runner
	decorate func(kubectl.Cmd) kubectl.Cmd
	version  *versionCache
}

// NewKubectl returns a Kubectl running r with every command decorated by
// decorate (the root's kubectl.Decorate for this session).
func NewKubectl(r Runner, decorate func(kubectl.Cmd) kubectl.Cmd) Kubectl {
	return Kubectl{runner: r, decorate: decorate, version: &versionCache{}}
}

func (k Kubectl) enabled() bool { return k.runner != nil && k.decorate != nil }

// describe runs cmd, the describe of the kind object called name,
// decorated, and returns its stdout and kubectl's version. A name kubectl
// would read as a flag ("--all") never reaches it; the runner checks the
// namespace. stderr is ignored when kubectl exits 0: it carries warnings,
// not the describe.
func (k Kubectl) describe(ctx context.Context, kind, name string, cmd kubectl.Cmd) (string, string, error) {
	if err := kubectl.CheckSubdomain(kind, name); err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, kubectlTimeout)
	defer cancel()
	res, err := k.runner.Run(ctx, k.decorate(cmd))
	if err != nil {
		return "", "", err
	}
	out := string(res.Stdout)
	if res.Truncated {
		out += fmt.Sprintf("\n… cut at %d MiB", kubectl.MaxStdout>>20)
	}
	return out, k.version.get(ctx, k.runner), nil
}

// versionCache holds kubectl's client version once it is known. A failed
// lookup is not kept, so a kubectl installed later is named.
type versionCache struct {
	mu      sync.Mutex
	version string
}

func (c *versionCache) get(ctx context.Context, r Runner) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != "" {
		return c.version
	}
	v, err := clientVersion(ctx, r)
	if err != nil {
		return ""
	}
	c.version = v
	return v
}

// clientVersion asks kubectl for its own version: no request reaches the
// cluster.
func clientVersion(ctx context.Context, r Runner) (string, error) {
	res, err := r.Run(ctx, kubectl.New("version", "--client", "-o", "json"))
	if err != nil {
		return "", err
	}
	var out struct {
		ClientVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"clientVersion"`
	}
	if err := json.Unmarshal(res.Stdout, &out); err != nil {
		return "", fmt.Errorf("decode kubectl version: %w", err)
	}
	v := []rune(sanitize.Line(out.ClientVersion.GitVersion))
	if len(v) > maxVersion {
		v = v[:maxVersion]
	}
	if len(v) == 0 {
		return "", errors.New("kubectl reports no version")
	}
	return string(v), nil
}

// kubectlOnlyError is a describe of a kind the built-in describer does
// not know, when kubectl could not describe it either: err is why, nil
// when the view was given no kubectl to run.
type kubectlOnlyError struct {
	kind Kind
	err  error
}

// Error says what the describe needs, as the body and the status show it.
func (e kubectlOnlyError) Error() string {
	what := "describe " + sanitize.Line(string(e.kind))
	switch {
	case e.err == nil:
		return what + " needs kubectl"
	case errors.Is(e.err, kubectl.ErrNotFound):
		return what + " needs kubectl (not found on PATH)"
	case errors.Is(e.err, context.DeadlineExceeded):
		return "kubectl " + what + " timed out"
	}
	return "kubectl " + what + " failed: " + sanitize.Line(e.err.Error())
}

func (e kubectlOnlyError) Unwrap() error { return e.err }

// fallbackNote is the status that says why the built-in describer runs
// instead of kubectl.
func fallbackNote(err error) (text string, missing bool) {
	var exit *kubectl.ExitError
	switch {
	case errors.Is(err, kubectl.ErrNotFound):
		return "kubectl not found on PATH: built-in describe", true
	case errors.Is(err, context.DeadlineExceeded):
		return "kubectl timed out: built-in describe", false
	case errors.As(err, &exit):
		return "built-in describe: " + exit.Error(), false
	}
	return "built-in describe: " + err.Error(), false
}
