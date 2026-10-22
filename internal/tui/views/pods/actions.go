package pods

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// noticeDelay is how long a resolve runs before the status line says so:
// long enough that a healthy API answers first and nothing flickers.
const noticeDelay = 150 * time.Millisecond

// action is a row action that looks the pod up before it asks the root
// for anything: its containers for logs and exec, that it still exists
// for port-forward.
type action int

const (
	actLogs action = iota
	actExec
	actForward
	actionCount
)

// key names the action in the InFlight set; for logs and exec it is also
// the container picker's NextKind.
func (a action) key() string {
	switch a {
	case actLogs:
		return "logs"
	case actExec:
		return "exec"
	}
	return "forward"
}

// label names the action in the status line.
func (a action) label() string {
	if a == actForward {
		return "port-forward"
	}
	return a.key()
}

// podTarget is the row an action was asked for.
type podTarget struct {
	namespace string
	name      string
	// windows is the listed pod's OS. A pod's OS never changes, so the
	// list says it as well as the lookup's GET would.
	windows bool
}

// shell is the shell a runs in the pod, as the picker's preview, the
// footer and the root's exec name it; none for anything but exec.
func (t podTarget) shell(a action) string {
	if a != actExec {
		return ""
	}
	return views.ShellFor(t.windows)
}

func (t podTarget) key() string { return views.RowKey(t.namespace, t.name) }

func (t podTarget) ref() views.ResourceRef {
	return views.ResourceRef{Kind: "pod", Namespace: t.namespace, Name: t.name}
}

// asked holds, per action, the row its request in flight was made for. An
// array is a value, so every copy of the view keeps its own.
type asked [actionCount]podTarget

// resolvingMsg fires noticeDelay after an action started.
type resolvingMsg struct {
	tok async.Token
	act action
}

// actionPod is the pod a row key acts on: the one under the cursor of a
// list that loaded.
func (m Model) actionPod() (k8s.Pod, tea.Cmd, bool) {
	switch {
	case m.client == nil:
		return k8s.Pod{}, statusCmd(status.Warn, "no kubeconfig"), false
	case !m.loaded:
		return k8s.Pod{}, statusCmd(status.Info, "pods are still loading"), false
	case m.health.Failed():
		return k8s.Pod{}, statusCmd(status.Warn, "pods unavailable, press r"), false
	}
	pod, ok := m.cursorPod()
	if !ok {
		return k8s.Pod{}, statusCmd(status.Warn, "no pod selected"), false
	}
	return pod, nil, true
}

func (m Model) describe() tea.Cmd {
	pod, failed, ok := m.actionPod()
	if !ok {
		return failed
	}
	return send(views.DescribeRequestMsg{Kind: "pod", Namespace: pod.Namespace, Name: pod.Name})
}

// startAction looks the cursor's pod up for a. Logs follow the last key
// pressed (another row's l replaces the lookup in flight, the same row's
// is ignored); exec and port-forward follow the first, so one key press
// opens one shell or one prompt, and port-forward waits for every other
// lookup too.
func (m Model) startAction(a action) (Model, tea.Cmd) {
	pod, failed, ok := m.actionPod()
	if !ok {
		return m, failed
	}
	target := podTarget{namespace: pod.Namespace, name: pod.Name, windows: pod.Windows}
	tok := m.tok.Next()
	acts, started := m.begin(a, target, tok)
	if !started {
		return m, nil
	}
	m.tok, m.acts = tok, acts
	m.asked[a] = target
	return m, tea.Batch(m.resolve(tok, target, a), m.noticeAfter(tok, a))
}

func (m Model) begin(a action, t podTarget, tok async.Token) (async.InFlight, bool) {
	switch a {
	case actLogs:
		return m.acts.Supersede(a.key(), t.key(), tok)
	case actForward:
		if m.resolving() {
			return m.acts, false
		}
	}
	return m.acts.Begin(a.key(), tok)
}

func (m Model) resolving() bool {
	for a := range actionCount {
		if m.acts.Busy(a.key()) {
			return true
		}
	}
	return false
}

// resolve looks the pod up for a with one GET. The reply is what a offers
// (offered).
func (m Model) resolve(tok async.Token, t podTarget, a action) tea.Cmd {
	c := m.client
	return async.Run(tok, async.DefaultTimeout, func(ctx context.Context) ([]string, error) {
		cs, err := c.PodContainers(ctx, t.namespace, t.name)
		if err != nil {
			return nil, err
		}
		return offered(a, cs), nil
	})
}

// initMark heads an init container's or native sidecar's label, as the
// container picker shows it. A container name is a DNS label, which has no
// colon, so a label reads back as one name only.
const initMark = "init:"

// offered labels the containers a offers, in picker order: logs offer
// every container, since a pod stuck in Init says why in its init
// containers' logs; exec offers the ones a shell runs in, the app
// containers and native sidecars; port-forward needs none.
func offered(a action, cs k8s.Containers) []string {
	switch a {
	case actLogs:
	case actExec:
		cs = cs.ForExec()
	default:
		return nil
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		if c.Init {
			out = append(out, initMark+c.Name)
			continue
		}
		out = append(out, c.Name)
	}
	return out
}

// unmark reads offered's labels back: every container's name, in order,
// and the names of those marked init.
func unmark(labels []string) (names, init []string) {
	names = make([]string, 0, len(labels))
	for _, l := range labels {
		name, marked := strings.CutPrefix(l, initMark)
		names = append(names, name)
		if marked {
			init = append(init, name)
		}
	}
	return names, init
}

func (m Model) noticeAfter(tok async.Token, a action) tea.Cmd {
	return m.clock.Tick(noticeDelay, func(time.Time) tea.Msg {
		return resolvingMsg{tok: tok, act: a}
	})
}

// onResolving says the action is still resolving, unless it was answered
// or replaced in the meantime.
func (m Model) onResolving(msg resolvingMsg) tea.Cmd {
	if !m.acts.Accept(msg.act.key(), msg.tok) {
		return nil
	}
	t := m.asked[msg.act]
	return statusCmd(status.Info, fmt.Sprintf("resolving %s for pod %s…", msg.act.label(), t.key()))
}

// onResolved takes the reply of the action it answers; a reply for a
// replaced lookup, or for another view, answers none and is dropped.
func (m Model) onResolved(msg async.Result[[]string]) (Model, tea.Cmd) {
	for a := range actionCount {
		acts, ok := m.acts.End(a.key(), msg.Token)
		if !ok {
			continue
		}
		m.acts = acts
		return m, m.dispatch(a, m.asked[a], msg)
	}
	return m, nil
}

// dispatch turns a lookup into the request it was for. A single container
// skips the picker. When the cursor has left the row meanwhile the prompt
// still opens, marked Detached, so the user sees whose logs they get.
func (m Model) dispatch(a action, t podTarget, res async.Result[[]string]) tea.Cmd {
	if res.Err != nil {
		return lookupFailed(a, t, res.Err)
	}
	if a == actForward {
		return send(views.ForwardRequestMsg{Kind: "pod", Namespace: t.namespace, Name: t.name})
	}
	containers, init := unmark(res.Value)
	if len(containers) == 0 {
		return statusCmd(status.Warn, "pod "+t.name+" has no containers")
	}
	cur, _ := m.cursorPod()
	detached := rowKey(cur) != t.key()
	pods := []string{t.name}
	switch {
	case len(containers) > 1:
		return send(views.ContainerPromptRequestMsg{
			Namespace: t.namespace, Pods: pods, Containers: containers, Init: init, NextKind: a.key(),
			Target: t.ref(), Detached: detached, Shell: t.shell(a),
		})
	case a == actLogs:
		return send(views.TailPromptRequestMsg{
			Namespace: t.namespace, Pods: pods, Container: containers[0],
			Target: t.ref(), Detached: detached,
		})
	}
	return send(views.ExecRequestMsg{Namespace: t.namespace, Pod: t.name, Container: containers[0], Shell: t.shell(a)})
}

func lookupFailed(a action, t podTarget, err error) tea.Cmd {
	if apierrors.IsNotFound(err) {
		return statusCmd(status.Warn, "pod "+t.name+" gone, press r")
	}
	return statusCmd(status.Error, fmt.Sprintf("%s for pod %s: %s", a.label(), t.key(), stale.Describe(err)))
}

func statusCmd(level status.Level, text string) tea.Cmd {
	return send(views.StatusMsg{Level: level, Text: text})
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
