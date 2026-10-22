package views

import (
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// LogsCommand is the kubectl command behind a logs stream of pods in ns,
// shared by the logs view and the prompts that lead to it so the footer
// shows the same command before and after the stream opens:
//
//   - one pod: `kubectl logs -f <pod> --tail=<n> -n <ns> [-c <c>]`, exact;
//   - several pods of a resolved target (t.Selector set): `kubectl logs -f
//     -l '<selector>' --prefix --tail=<n> --max-log-requests=<m> ...`,
//     where a deployment's selector is narrowed to t.Hashes and m covers
//     t.Matched() up to kubectl.MaxLogRequests; Approx when t.Approx() or
//     the selector matches more pods than are streamed;
//   - several pods without a target: kubectl cannot follow a list of pod
//     names, so the first pod's command, marked Approx.
//
// Anything kubectl would reject (no pods, an invalid name) gives the zero
// Cmd, which the footer renders as an empty row.
func LogsCommand(ns string, pods []string, container string, tail int64, t k8s.LogTarget) kubectl.Cmd {
	switch {
	case len(pods) == 1:
		return orZero(kubectl.PodLogs(ns, pods[0], container, tail))
	case t.Selector != "":
		return selectorLogs(ns, len(pods), container, tail, t)
	case len(pods) > 1:
		c := orZero(kubectl.PodLogs(ns, pods[0], container, tail))
		c.Approx = !c.IsZero()
		return c
	}
	return kubectl.Cmd{}
}

// StreamedPods is how many of n pods a logs view streams at once: all of
// them, up to kubectl.MaxLogRequests. A tail is shared out among those
// (ClampTail(tail, StreamedPods(n))), so the prompts and the logs view
// agree on it.
func StreamedPods(n int) int {
	return min(n, kubectl.MaxLogRequests)
}

func selectorLogs(ns string, streamed int, container string, tail int64, t k8s.LogTarget) kubectl.Cmd {
	selector := t.Selector
	if t.Kind == k8s.LogKindDeployment {
		selector = kubectl.HashSelector(t.Selector, t.Hashes)
	}
	c := orZero(kubectl.SelectorLogs(kubectl.LogsSpec{
		Namespace: ns,
		Selector:  selector,
		Container: container,
		Tail:      tail,
		Matched:   t.Matched(),
		Streamed:  streamed,
	}))
	c.Approx = c.Approx || (t.Approx() && !c.IsZero())
	return c
}

const (
	// ExecShell is the shell `e` runs in a container, WindowsShell in a
	// Windows pod's, which has no sh. The footer preview and the exec the
	// root starts use the same argv.
	ExecShell = "sh"
	// WindowsShell is the shell `e` runs in a Windows pod.
	WindowsShell = "cmd.exe"
)

// ShellFor is the shell `e` runs in a pod: WindowsShell on Windows,
// ExecShell anywhere else.
func ShellFor(windows bool) string {
	if windows {
		return WindowsShell
	}
	return ExecShell
}

// ExecCommand is `kubectl exec -it -n <ns> [-c <c>] <pod> -- <shell>` for
// req, with req.Shell, or ExecShell when the request names none. It fails
// on a name kubectl would misread, such as a pod named like a flag.
func ExecCommand(req ExecRequestMsg) (kubectl.Cmd, error) {
	shell := req.Shell
	if shell == "" {
		shell = ExecShell
	}
	return kubectl.Exec(req.Namespace, req.Pod, req.Container, shell)
}

// orZero drops a builder error: an invalid command is not shown at all.
func orZero(c kubectl.Cmd, err error) kubectl.Cmd {
	if err != nil {
		return kubectl.Cmd{}
	}
	return c
}
