package startup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// errNoKubeconfig reports that no kubeconfig file exists anywhere k4s looks.
var errNoKubeconfig = errors.New("no kubeconfig found: pass --kubeconfig, set KUBECONFIG or create ~/.kube/config")

// resolveSource picks the kubeconfig: --kubeconfig, then $KUBECONFIG, then
// ~/.kube/config, then the k3s server's file. The last one is k4s's own
// addition, so that k4s works on a k3s node without setup; upstream kubectl
// never reads it.
func resolveSource(explicit string, env Env) (k8s.Source, error) {
	if explicit != "" {
		src := k8s.Source{Paths: []string{explicit}, Origin: k8s.OriginFlag}
		found, err := probe(explicit)
		if err != nil {
			return k8s.Source{}, newReadError(explicit, src.Origin, env, err)
		}
		if !found {
			return k8s.Source{}, fmt.Errorf("kubeconfig %s: %w", explicit, fs.ErrNotExist)
		}
		return src, nil
	}
	if list := env.Getenv("KUBECONFIG"); list != "" {
		return kubeconfigList(list, env)
	}
	for _, c := range []k8s.Source{
		{Paths: []string{homeKubeconfig(env.Home)}},
		{Paths: []string{env.K3sKubeconfig}, Origin: k8s.OriginK3s},
	} {
		path := c.Paths[0]
		if path == "" {
			continue
		}
		found, err := probe(path)
		if err != nil {
			return k8s.Source{}, newReadError(path, c.Origin, env, err)
		}
		if found {
			return c, nil
		}
	}
	return k8s.Source{}, errNoKubeconfig
}

func homeKubeconfig(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// kubeconfigList splits $KUBECONFIG the way client-go does: blanks and
// repeats are dropped and missing files skipped, but one file must exist.
func kubeconfigList(list string, env Env) (k8s.Source, error) {
	var paths []string
	for _, p := range filepath.SplitList(list) {
		if p != "" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	src := k8s.Source{Paths: paths}
	found := false
	for _, p := range paths {
		ok, err := probe(p)
		if err != nil {
			return k8s.Source{}, newReadError(p, src.Origin, env, err)
		}
		found = found || ok
	}
	if !found {
		return src, fmt.Errorf("%w (none of the KUBECONFIG files exist: %s)", errNoKubeconfig, strings.Join(paths, ", "))
	}
	return src, nil
}

// probe reports whether path exists and can be opened. A missing file is
// not an error; anything else that stops reading it is.
func probe(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = f.Close()
	return true, nil
}

// readError is a kubeconfig that exists but cannot be read. k4s exits on
// it: client-go would fail on the same file, so the TUI could not even
// switch contexts.
type readError struct {
	err error
	fix string
}

func newReadError(path string, origin k8s.Origin, env Env, err error) *readError {
	re := &readError{err: err}
	if errors.Is(err, fs.ErrPermission) && path == env.K3sKubeconfig {
		re.fix = k3sFix(origin, hasHomeKubeconfig(env.Home))
	}
	return re
}

// hasHomeKubeconfig reports whether ~/.kube/config is there, or might be:
// a file that cannot even be probed counts, so advice never overwrites it.
func hasHomeKubeconfig(home string) bool {
	path := homeKubeconfig(home)
	if path == "" {
		return false
	}
	found, err := probe(path)
	return found || err != nil
}

func (e *readError) Error() string {
	if e.fix == "" {
		return e.err.Error()
	}
	return e.err.Error() + "\n" + e.fix
}

func (e *readError) Unwrap() error { return e.err }

// k3sFix explains how a non-root user gets a readable copy of the k3s
// admin kubeconfig, which k3s writes with mode 0600 for root. The copy is
// made under umask 077, so it is never readable by others, not even
// between its creation and a chmod. It goes to ~/.kube/config only when
// haveConfig is false: the fallback to the k3s file runs only then, but
// --kubeconfig and KUBECONFIG can name the file beside an existing one,
// which the copy must not overwrite.
func k3sFix(origin k8s.Origin, haveConfig bool) string {
	target, then := "~/.kube/config", k3sThen(origin)
	if haveConfig {
		target, then = "~/.kube/k3s.yaml", k3sThenSide(origin)
	}
	lines := []string{
		"  k3s writes this file for root only; copy it for your user:",
		"    mkdir -p -m 700 ~/.kube && (umask 077 && sudo k3s kubectl config view --raw > " + target + ")",
		"  or start the k3s server with --write-kubeconfig-mode 644, which lets every local user read it",
	}
	if then != "" {
		lines = append(lines, then)
	}
	return strings.Join(lines, "\n")
}

// k3sThen is the step after a copy to ~/.kube/config, which kubectl and
// k4s read by default.
func k3sThen(origin k8s.Origin) string {
	switch origin {
	case k8s.OriginFlag:
		return "  then start k4s without --kubeconfig"
	case k8s.OriginKubectl:
		return "  then unset KUBECONFIG, or point it at ~/.kube/config"
	}
	return ""
}

// k3sThenSide is the step after a copy to ~/.kube/k3s.yaml, which nothing
// reads unless told to.
func k3sThenSide(origin k8s.Origin) string {
	if origin == k8s.OriginKubectl {
		return "  then point KUBECONFIG at ~/.kube/k3s.yaml"
	}
	return "  then start k4s with --kubeconfig ~/.kube/k3s.yaml"
}
