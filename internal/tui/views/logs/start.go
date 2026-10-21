package logs

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// restartsUnknown marks a stream whose container's restart count was not
// known when it opened: a restart cannot be told from a drop until a GET
// has read it.
const restartsUnknown = -1

// baselineMsg answers the GET that reads the restart counts of pods
// opened without their state.
type baselineMsg struct {
	tok    async.Token
	checks []podCheck
}

// begin opens the first streams on a new session: the pods given, up to
// maxStreams. A pod of the target that has not started yet says so and
// waits. Init then sends the first re-resolve's tick, the GET of the
// restart counts the target did not carry, and the status of the pods
// left out.
func (m Model) begin(pods []string) Model {
	streamed := pods[:min(len(pods), maxStreams)]
	m.pods = streamed
	m.streams = tracker{attempts: m.streams.attempts, ends: m.streams.ends}
	m.explain = explainer{gen: m.explain.gen}
	if m.src == nil {
		m.mint = m.mint.Next()
		m.sess = session{tok: m.mint}
		return m
	}
	refs := make(map[string]k8s.PodRef, len(m.target.Pods))
	for _, ref := range m.target.Pods {
		refs[ref.Name] = ref
	}
	var reqs []attachReq
	var opened, unknown []string
	for _, pod := range streamed {
		s := podStream{container: m.container, restarts: restartsUnknown}
		ref, known := refs[pod]
		if known {
			s.container, s.created = m.containerFor(ref), ref.Created
			c, _ := ref.State.Container(s.container)
			s.restarts = c.Restarts
			if v := judge(end{found: true, state: ref.State, container: s.container, waiting: true}); v.outcome == outWaiting {
				m.appendOne(pod, v.note, lineNotice)
				s.phase, s.note = phaseWaiting, v.note
				m.streams = m.streams.with(pod, s)
				continue
			}
		} else {
			unknown = append(unknown, pod)
		}
		next, req := m.streams.open(pod, s, k8s.LogOptions{Tail: m.tail}, time.Time{})
		m.streams, reqs, opened = next, append(reqs, req), append(opened, pod)
	}
	mode := pumpDirect
	if m.target.Selector != "" || len(pods) > 1 {
		mode = pumpMerged
	}
	m.mint = m.mint.Next()
	m.sess = startSession(m.mint, m.src, m.namespace, m.crash, mode, opened, m.tail)
	for _, r := range reqs {
		m.sess.attach(r)
	}
	var tick tea.Cmd
	m.mint = m.mint.Next()
	m.follow, tick = m.newFollow(m.mint)
	if len(pods) > 1 {
		// A merge of several pods never asks for each one; a restart
		// there reads as a drop and is reattached all the same.
		unknown = nil
	}
	m.startup = tea.Batch(tick, m.baselineCmd(unknown), m.overCmd(len(pods)))
	if len(m.rawLines) > 0 {
		m.syncViewport()
	}
	return m
}

// overCmd says how many pods are left out past maxStreams, if any.
func (m *Model) overCmd(total int) tea.Cmd {
	if total <= maxStreams {
		return nil
	}
	m.join.shownOver = total
	return statusCmd(status.Warn, overText(total))
}

// baselineCmd reads the restart counts of pods, one GET each.
func (m Model) baselineCmd(pods []string) tea.Cmd {
	if len(pods) == 0 || m.res == nil {
		return nil
	}
	res, ns, tok := m.res, m.namespace, m.sess.tok
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), async.DefaultTimeout)
		defer cancel()
		checks := make([]podCheck, 0, len(pods))
		for _, pod := range pods {
			st, err := res.PodState(ctx, ns, pod)
			checks = append(checks, podCheck{pod: pod, state: st, found: err == nil, err: err})
		}
		return baselineMsg{tok: tok, checks: checks}
	}
}

func (m Model) onBaseline(msg baselineMsg) (views.View, tea.Cmd) {
	if !m.sess.tok.Accept(msg.tok) {
		return m, nil
	}
	for _, c := range msg.checks {
		s, ok := m.streams.pods[c.pod]
		if !ok || !c.found || s.restarts != restartsUnknown {
			continue
		}
		if ct, ok := c.state.Container(s.container); ok {
			s.restarts = ct.Restarts
			m.streams = m.streams.with(c.pod, s)
		}
	}
	return m, nil
}

// onEnd takes the end of one reader. In a view with a client the reason
// waits for the next check (see explainer); without one, or for the
// previous instance's log, it is said at once. ok reports an end the
// check must explain.
func (m *Model) onEnd(ev logEvent) (tea.Cmd, bool) {
	var said tea.Cmd
	if internal := (internalError{}); errors.As(ev.Err, &internal) {
		said = statusCmd(status.Error, internal.Error())
	}
	switch {
	case ev.Pod == "":
		m.appendOne("", sanitize.Line(ev.Err.Error()), lineStreamErr)
		return said, false
	case m.previous && ev.Err != nil:
		m.appendOne(ev.Pod, errText(ev.Err), lineStreamErr)
		return said, false
	case m.previous:
		m.appendOne(ev.Pod, notePrevious, lineNotice)
		return said, false
	}
	next, ok := m.streams.ended(ev.Pod, ev.Attempt, ev.Err)
	if !ok {
		return said, false
	}
	m.streams = next
	if m.res != nil {
		return said, true
	}
	s := m.streams.pods[ev.Pod]
	s.phase = phaseDone
	m.streams = m.streams.with(ev.Pod, s)
	if ev.Err != nil {
		m.appendOne(ev.Pod, noteEndedHead+errText(ev.Err), lineAlert)
		return said, false
	}
	m.appendOne(ev.Pod, "○ ended", lineNotice)
	return said, false
}
