package logs

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// requestsFlag is the kubectl flag that bounds a follow of several pods.
const requestsFlag = "--max-log-requests="

// Title implements views.View.
func (m Model) Title() string {
	base := "logs"
	if len(m.pods) == 1 {
		base += " · " + m.pods[0]
	} else {
		base += fmt.Sprintf(" · %d pods", len(m.pods))
	}
	if m.container != "" {
		base += " / " + m.container
	}
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

// Command implements views.View: the kubectl logs command for the pods
// streamed now, which changes as pods join and leave, with --previous
// instead of -f while the previous instance shows. k4s never streams more
// than maxStreams pods, and says so: past that the command asks for that
// many requests and is marked approximate.
func (m Model) Command() kubectl.Cmd {
	c := capRequests(views.LogsCommand(m.namespace, m.pods, m.container, m.tail, m.target), maxStreams)
	if m.previous {
		return previousCommand(c)
	}
	return c
}

// capRequests lowers c's --max-log-requests to limit, marking c
// approximate when it does.
func capRequests(c kubectl.Cmd, limit int) kubectl.Cmd {
	args := make([]string, len(c.Args))
	capped := false
	for i, a := range c.Args {
		args[i] = a
		n, err := strconv.Atoi(strings.TrimPrefix(a, requestsFlag))
		if strings.HasPrefix(a, requestsFlag) && err == nil && n > limit {
			args[i] = requestsFlag + strconv.Itoa(limit)
			capped = true
		}
	}
	if !capped {
		return c
	}
	out := kubectl.New(args...)
	out.Approx = true
	return out
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
