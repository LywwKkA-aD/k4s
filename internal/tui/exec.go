package tui

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// execDoneMsg is delivered by tea.ExecProcess after the kubectl-exec shell
// returns control to the TUI.
type execDoneMsg struct{ err error }

// onExecRequest hands the terminal to `kubectl exec -it` through
// tea.ExecProcess; when the shell exits an execDoneMsg comes back. A
// missing kubectl ("kubectl not found on PATH") or a name kubectl would
// misread lands in the status row instead.
func (m Model) onExecRequest(msg views.ExecRequestMsg) (tea.Model, tea.Cmd) {
	c, err := m.execCommand(msg)
	if err != nil {
		return m.setError("exec: " + err.Error())
	}
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// execCommand is the exec command the footer would show for req, decorated
// for this cluster, as the argv the runner starts: no shell in between,
// and KUBECONFIG pointed at the files the TUI loaded, so the k3s multicall
// kubectl cannot wander off to /etc/rancher/k3s/k3s.yaml.
func (m Model) execCommand(req views.ExecRequestMsg) (*exec.Cmd, error) {
	cmd, err := views.ExecCommand(req)
	if err != nil {
		return nil, err
	}
	return m.runner.Command(m.decorate(cmd))
}

// onExecDone handles the return of the kubectl exec subprocess.
func (m Model) onExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.setError("exec: " + msg.err.Error())
	}
	return m, nil
}

func kubeconfigPaths(c *k8s.Client) []string {
	if c == nil {
		return nil
	}
	return c.KubeconfigPaths
}
