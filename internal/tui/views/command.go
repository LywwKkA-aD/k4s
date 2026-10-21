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
//     t.Matched(); Approx when t.Approx() or the selector matches more pods
//     than are streamed;
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

// ExecShell is the command `e` runs in a container. The footer preview and
// the exec the root starts use the same argv.
const ExecShell = "sh"

// ExecCommand is `kubectl exec -it -n <ns> [-c <c>] <pod> -- sh` for req.
// It fails on a name kubectl would misread, such as a pod named like a
// flag.
func ExecCommand(req ExecRequestMsg) (kubectl.Cmd, error) {
	return kubectl.Exec(req.Namespace, req.Pod, req.Container, ExecShell)
}

// orZero drops a builder error: an invalid command is not shown at all.
func orZero(c kubectl.Cmd, err error) kubectl.Cmd {
	if err != nil {
		return kubectl.Cmd{}
	}
	return c
}
