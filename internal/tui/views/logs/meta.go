package logs

import (
	"fmt"

	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Title implements views.View: what the view streams, named as the
// other object views name theirs ("pod · web-1 · yaml"): "pod · web-1 /
// nginx · logs" for a pod, "deployment · web · logs · 3 pods" for the
// pods of a deployment or a service (the pod by name when there is one),
// then the instance, the search and the pause.
func (m Model) Title() string {
	base := m.subject()
	if m.previous {
		base += " · previous"
	}
	if m.search.text != "" {
		if len(m.matches) > 0 {
			base += fmt.Sprintf(" · /%s · %d/%d", m.search.text, m.matchIdx+1, len(m.matches))
		} else {
			base += " · /" + m.search.text + " · no match"
		}
	}
	if !m.autoFollow {
		base += " · paused"
	}
	return base
}

// subject names the object streamed, its names made inert.
func (m Model) subject() string {
	container := ""
	if m.container != "" {
		container = " / " + sanitize.Line(m.container)
	}
	switch {
	case m.target.Kind != "" && m.target.Name != "":
		return sanitize.Line(m.target.Kind) + " · " + sanitize.Line(m.target.Name) + " · logs · " + m.podsLabel() + container
	case len(m.pods) == 1:
		return "pod · " + sanitize.Line(m.pods[0]) + container + " · logs"
	}
	return "logs · " + m.podsLabel() + container
}

// podsLabel is the one pod streamed by name, else how many there are.
func (m Model) podsLabel() string {
	if len(m.pods) == 1 {
		return sanitize.Line(m.pods[0])
	}
	return fmt.Sprintf("%d pods", len(m.pods))
}

// Command implements views.View: the kubectl logs command for the pods
// streamed now, which changes as pods join and leave, with --previous
// instead of -f while the previous instance shows. k4s never streams more
// than maxStreams pods, and says so: past that the command asks for that
// many requests and is marked approximate (views.LogsCommand), as the
// prompts before the view previewed it.
func (m Model) Command() kubectl.Cmd {
	c := views.LogsCommand(m.namespace, m.pods, m.container, m.tail, m.target)
	if m.previous {
		return previousCommand(c)
	}
	return c
}

// Help implements views.View. 't' is only advertised when there is more
// than one pod and 'p' only for one, where each does something. The
// horizontal-scroll keys only appear when wrap is off, since they are a
// no-op in wrap mode. A view that follows its pods says so last.
func (m Model) Help() []key.Binding {
	k := m.keys
	bindings := []key.Binding{k.scroll, k.follow, k.wrap}
	if !m.wrap {
		bindings = append(bindings, k.left, k.right)
	}
	if m.multi {
		bindings = append(bindings, k.tags)
	}
	bindings = append(bindings, k.searchOpen, k.next, k.prev, k.clear)
	if len(m.pods) == 1 {
		bindings = append(bindings, m.previousKey())
	}
	if m.following() {
		bindings = append(bindings, k.newPods)
	}
	return bindings
}

// CapturesKeys implements views.View. Returns true while the search prompt
// is focused so that 'q' / ':' reach the input as text rather than the
// global navigation.
func (m Model) CapturesKeys() bool { return m.searchOpen }

// Close cancels the streams, and a previous-instance request in flight,
// draining the goroutines. A closed view starts nothing more: no
// re-resolve, check or reconnect. Idempotent.
func (m Model) Close() error {
	m.sess.stop()
	m.opening.stop()
	return nil
}

var _ views.View = Model{}
