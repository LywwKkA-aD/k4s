package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	kubectlBin   = "kubectl"
	kubeconfigEV = "KUBECONFIG"
)

// errKubectlMissing replaces the bare "exit status 1" a missing kubectl
// would otherwise leave in the footer.
var errKubectlMissing = errors.New("kubectl not found on PATH")

// execDoneMsg is delivered by tea.ExecProcess after the kubectl-exec shell
// returns control to the TUI.
type execDoneMsg struct{ err error }

// onExecRequest runs `kubectl exec -it` against the named pod / container
// via tea.ExecProcess so the shell takes over the terminal cleanly. When
// the shell exits we get an execDoneMsg back at the root.
func (m Model) onExecRequest(msg views.ExecRequestMsg) (tea.Model, tea.Cmd) {
	cmd, err := newExecCommand(exec.LookPath, m.client, msg)
	if err != nil {
		return m.setCmdError("exec: " + err.Error())
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// onExecDone handles the return of the kubectl exec subprocess.
func (m Model) onExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.setCmdError("exec: " + msg.err.Error())
	}
	return m, nil
}

// newExecCommand checks that kubectl is installed and builds the exec
// command pointed at the kubeconfig files k4s itself loaded. Without the
// explicit KUBECONFIG the k3s multicall kubectl reads
// /etc/rancher/k3s/k3s.yaml first and can land on a different cluster than
// the one the TUI shows.
func newExecCommand(lookPath func(string) (string, error), client *k8s.Client, req views.ExecRequestMsg) (*exec.Cmd, error) {
	if _, err := lookPath(kubectlBin); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errKubectlMissing
		}
		return nil, fmt.Errorf("find kubectl: %w", err)
	}
	cmd := buildExecCommand(currentContextName(client), req.Namespace, req.Pod, req.Container)
	cmd.Env = execEnv(os.Environ(), kubeconfigPaths(client))
	return cmd, nil
}

// buildExecCommand returns the kubectl exec command. Extracted so the args
// are unit-testable without spawning a subprocess.
//
// gosec G204: exec.Command with variable args is normally a red flag, but
// here every variable comes from a Kubernetes API object (namespace, pod,
// container names that the user already had RBAC to read). None of it
// originates from a free-form text field — even the container name is
// matched back against the API-supplied list before this is called.
func buildExecCommand(contextName, namespace, pod, container string) *exec.Cmd {
	args := []string{"exec", "-it"}
	if contextName != "" {
		args = append(args, "--context", contextName)
	}
	args = append(args, "-n", namespace)
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, pod, "--", "sh")
	return exec.Command(kubectlBin, args...) //nolint:gosec
}

// execEnv returns a copy of base with KUBECONFIG set to paths in loading
// order. A nil result means "inherit the parent environment", which is what
// we want when k4s has no recorded kubeconfig paths.
func execEnv(base, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := withoutEnvKey(base, kubeconfigEV, runtime.GOOS == "windows")
	return append(out, kubeconfigEV+"="+strings.Join(paths, string(os.PathListSeparator)))
}

// withoutEnvKey returns a copy of env without key. Windows environment
// names are case-insensitive, so foldCase drops "Kubeconfig=" there too.
func withoutEnvKey(env []string, key string, foldCase bool) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == key || (foldCase && strings.EqualFold(name, key)) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func kubeconfigPaths(c *k8s.Client) []string {
	if c == nil {
		return nil
	}
	return c.KubeconfigPaths
}
