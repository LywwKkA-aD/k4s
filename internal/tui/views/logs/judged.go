package logs

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// stateOf looks a pod up in a check: its state, whether it exists, and
// whether the check could tell at all.
type stateOf func(pod string) (state k8s.PodState, found, known bool)

// judgeEnded settles every stream that ended by upTo, and in a following
// view every pod that waits, from the states lookup gives.
func (m Model) judgeEnded(upTo uint64, lookup stateOf) (Model, tea.Cmd) {
	pods := m.streams.endedBy(upTo)
	if m.following() {
		pods = append(pods, m.streams.inPhase(phaseWaiting)...)
	}
	var cmds []tea.Cmd
	var reqs []attachReq
	for _, pod := range pods {
		s := m.streams.pods[pod]
		st, found, known := lookup(pod)
		v := verdict{outcome: outDropped, note: noteDropped, restarts: s.restarts}
		if known {
			v = judge(end{found: found, state: st, container: s.container, restarts: s.restarts, err: s.err, waiting: !s.streamed})
		}
		var cmd tea.Cmd
		var req *attachReq
		m, cmd, req = m.settlePod(pod, s, v)
		cmds = append(cmds, cmd)
		if req != nil {
			reqs = append(reqs, *req)
		}
	}
	return m, tea.Batch(append(cmds, attachCmd(m.sess, reqs))...)
}

// settlePod acts on v for pod: a notice, then the pod ends, waits, retries
// or opens a reader, whose request it returns.
func (m Model) settlePod(pod string, s podStream, v verdict) (Model, tea.Cmd, *attachReq) {
	switch v.outcome {
	case outRestarted, outStarted:
		if v.note != "" {
			m.appendOne(pod, v.note, lineNotice)
		}
		s.restarts, s.drops = v.restarts, 0
		opts := k8s.LogOptions{Tail: m.tail}
		if v.outcome == outStarted {
			opts.Since = s.created
		}
		// A new instance's log is its own: nothing in it was shown yet.
		s.resume = time.Time{}
		next, req := m.streams.open(pod, s, opts, time.Time{})
		m.streams = next
		return m, nil, &req
	case outExited, outWaiting:
		if v.note != s.note {
			m.appendOne(pod, v.note, lineNotice)
			s.note = v.note
		}
		s.phase = phaseWaiting
		// A container that never started has no log yet, whatever a
		// reader tried: once it starts, it is read from the beginning.
		s.streamed = s.streamed && v.outcome == outExited
		if m.following() {
			m.streams = m.streams.with(pod, s)
			return m, nil, nil
		}
		s.drops++
		m.streams = m.streams.with(pod, s)
		return m, m.after(retryDelay(s.drops-1), recheckMsg{tok: m.sess.tok, pod: pod, attempt: s.attempt}), nil
	case outDropped:
		s.phase = phaseRetry
		s.drops++
		m.streams = m.streams.with(pod, s)
		return m, m.after(retryDelay(s.drops-1), retryMsg{tok: m.sess.tok, pod: pod, attempt: s.attempt}), nil
	}
	kind := lineNotice
	if v.outcome == outError {
		kind = lineAlert
	}
	m.appendOne(pod, v.note, kind)
	s.phase = phaseDone
	m.streams = m.streams.with(pod, s)
	return m, nil, nil
}

// after delivers msg once d has passed on the view's clock.
func (m Model) after(d time.Duration, msg tea.Msg) tea.Cmd {
	return m.clock.Tick(d, func(time.Time) tea.Msg { return msg })
}

// afterNotices lays out the notices appended since line first.
func (m *Model) afterNotices(first int) {
	if len(m.rawLines) == first {
		return
	}
	m.matches = append(m.matches, matchesFrom(m.search, m.rawLines, first)...)
	m.settle(first)
}
