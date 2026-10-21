package logs

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// followInterval is how often a deployment's or a service's pods are
	// resolved again: six pod lists a minute for an open view, none once
	// it is closed. No watch, no informer.
	followInterval = 10 * time.Second
	// explainDebounce gathers the ends of streams that end together, a
	// node's for one, under one check.
	explainDebounce = time.Second
	// checkSpacing is the least time between two checks of a view that does
	// not follow its pods: the ends in between wait for the next one, so a
	// stream that drops every 2s costs six GETs a minute, as many as a
	// following view's re-resolves cost Lists.
	checkSpacing = followInterval
	// maxStreams bounds the pods streamed at once, which is also the
	// --max-log-requests the footer asks for.
	maxStreams = kubectl.MaxLogRequests
	// followNote is what a following view says, in its help and on the
	// first pod that joins.
	followNote = "k4s re-resolves every 10s; kubectl logs -l does not follow new pods"
)

// resolver is the subset of *k8s.Client that follows a log target and
// reads the state of a pod whose stream ended.
type resolver interface {
	ReresolveLogTarget(ctx context.Context, t k8s.LogTarget) (k8s.LogTarget, error)
	PodState(ctx context.Context, namespace, name string) (k8s.PodState, error)
}

// compile-time assertion: *k8s.Client satisfies the resolver interface.
var _ resolver = (*k8s.Client)(nil)

// explainer tells why streams ended. A following view leaves that to its
// next re-resolve, every followInterval: pulling one forward would cost
// more than the six Lists a minute M5.7 allows. A view that does not
// follow its pods reads them, one GET each, in a check that runs
// explainDebounce after an end, so ends that come together share it, and
// no sooner than checkSpacing after the check before it.
type explainer struct {
	// armed is set while the debounce, or the spacing after a check, runs;
	// gen numbers it.
	armed bool
	gen   uint64
	// tok is the GET check in flight, zero for none.
	tok   async.Token
	busy  bool
	again bool // ends came while a check was in flight
	// upTo is the last end the check in flight covers.
	upTo uint64
	// failing is set while re-resolving fails, so the status says it once.
	failing bool
}

// podCheck is one pod's state as a GET check read it.
type podCheck struct {
	pod   string
	state k8s.PodState
	found bool
	err   error
}

type (
	// explainDueMsg is the debounce of a stream's end, or the spacing
	// after a check, running out.
	explainDueMsg struct {
		tok async.Token
		gen uint64
	}
	// retryMsg is a dropped stream's backoff being up.
	retryMsg struct {
		tok     async.Token
		pod     string
		attempt uint64
	}
	// recheckMsg is the time to look again at a pod that waits, in a view
	// that does not follow its pods.
	recheckMsg struct {
		tok     async.Token
		pod     string
		attempt uint64
	}
)

// following reports a view that re-resolves its pods: the logs of a
// deployment or a service, with a client.
func (m Model) following() bool {
	return m.res != nil && m.target.Selector != "" && !m.previous
}

// newFollow is the 10s re-resolve under tok, armed for a following view.
func (m Model) newFollow(tok async.Token) (async.Poller, tea.Cmd) {
	poll := async.NewPoller(tok, async.Config{Interval: followInterval, Clock: m.clock})
	if !m.following() {
		return poll, nil
	}
	return poll.WithEnabled(true)
}

func (m Model) onFollowTick(msg async.TickMsg) (views.View, tea.Cmd) {
	if !m.sess.live() || !m.following() {
		return m, nil
	}
	poll, ok := m.follow.Due(msg)
	if !ok {
		return m, nil
	}
	m.follow = poll
	return m.reresolve()
}

// reresolve lists the target's pods again; the reply explains every
// stream that ended before it was asked.
func (m Model) reresolve() (Model, tea.Cmd) {
	m.explain.upTo = m.streams.ends
	res, t := m.res, m.target
	return m, async.Run(m.follow.Token(), m.follow.Timeout(), func(ctx context.Context) (k8s.LogTarget, error) {
		return res.ReresolveLogTarget(ctx, t)
	})
}

func (m Model) onReresolved(msg async.Result[k8s.LogTarget]) (views.View, tea.Cmd) {
	if !m.follow.Accept(msg.Token) {
		return m, nil
	}
	poll, tick := m.follow.Done(msg.Token, msg.Err)
	m.follow = poll
	if !m.sess.live() || !m.following() {
		return m, nil
	}
	if msg.Err != nil {
		said := m.explain.failing
		m.explain.failing = true
		if said {
			return m, tick
		}
		return m, tea.Batch(tick, statusCmd(status.Warn, "logs of "+m.target.Kind+" "+m.target.Name+
			": cannot list its pods again: "+errText(msg.Err)))
	}
	m.explain.failing = false
	first := len(m.rawLines)
	m, cmd := m.applyTarget(msg.Value, m.explain.upTo)
	m.afterNotices(first)
	return m, tea.Batch(tick, cmd)
}

// armExplain sets the debounce that checks why streams ended, unless the
// debounce or the spacing after a check already runs: its end checks
// them all. A following view has its re-resolve for that.
func (m Model) armExplain() (Model, tea.Cmd) {
	if m.explain.armed || !m.sess.live() || m.following() {
		return m, nil
	}
	return m.armCheck(explainDebounce)
}

// armCheck runs the check due after d.
func (m Model) armCheck(d time.Duration) (Model, tea.Cmd) {
	m.explain.armed = true
	m.explain.gen++
	tok, gen := m.sess.tok, m.explain.gen
	return m, m.clock.Tick(d, func(time.Time) tea.Msg { return explainDueMsg{tok: tok, gen: gen} })
}

// explainLeft arms the next check when ends came while one was in flight.
func (m Model) explainLeft() (Model, tea.Cmd) {
	if !m.explain.again && !m.streams.unexplained(m.explain.upTo) {
		return m, nil
	}
	m.explain.again = false
	return m.armExplain()
}

func (m Model) onExplainDue(msg explainDueMsg) (views.View, tea.Cmd) {
	if !m.sess.tok.Accept(msg.tok) || !m.explain.armed || msg.gen != m.explain.gen {
		return m, nil
	}
	m.explain.armed = false
	if !m.sess.live() || m.following() {
		return m, nil
	}
	return m.startCheck()
}

// startCheck reads, one GET each, the state of every pod whose stream
// ended: the check of a view that does not follow its pods. The next one
// waits checkSpacing.
func (m Model) startCheck() (Model, tea.Cmd) {
	pods := m.streams.endedBy(m.streams.ends)
	if len(pods) == 0 || m.res == nil {
		return m, nil
	}
	if m.explain.busy {
		m.explain.again = true
		return m, nil
	}
	m.mint = m.mint.Next()
	m.explain.tok, m.explain.busy, m.explain.upTo = m.mint, true, m.streams.ends
	m, spacing := m.armCheck(checkSpacing)
	return m, tea.Batch(spacing, m.readStates(pods))
}

// readStates is the GET of each pod's state.
func (m Model) readStates(pods []string) tea.Cmd {
	res, ns := m.res, m.namespace
	return async.Run(m.explain.tok, async.DefaultTimeout, func(ctx context.Context) ([]podCheck, error) {
		out := make([]podCheck, 0, len(pods))
		for _, pod := range pods {
			st, err := res.PodState(ctx, ns, pod)
			check := podCheck{pod: pod, state: st, found: err == nil}
			if err != nil && !apierrors.IsNotFound(err) {
				check.err = err
			}
			out = append(out, check)
		}
		return out, nil
	})
}

func (m Model) onChecked(msg async.Result[[]podCheck]) (views.View, tea.Cmd) {
	if !m.explain.busy || !m.explain.tok.Accept(msg.Token) {
		return m, nil
	}
	m.explain.busy = false
	if !m.sess.live() || m.previous {
		return m, nil
	}
	checks := make(map[string]podCheck, len(msg.Value))
	for _, c := range msg.Value {
		checks[c.pod] = c
	}
	first := len(m.rawLines)
	m, cmd := m.judgeEnded(m.explain.upTo, func(pod string) (k8s.PodState, bool, bool) {
		c, ok := checks[pod]
		if !ok || c.err != nil || msg.Err != nil {
			return k8s.PodState{}, false, false
		}
		return c.state, c.found, true
	})
	m.afterNotices(first)
	m, more := m.explainLeft()
	return m, tea.Batch(cmd, more)
}

func (m Model) onRetry(msg retryMsg) (views.View, tea.Cmd) {
	s, ok := m.streams.pods[msg.pod]
	if !m.sess.tok.Accept(msg.tok) || !ok || s.phase != phaseRetry || s.attempt != msg.attempt || !m.sess.live() {
		return m, nil
	}
	opts := k8s.LogOptions{Since: s.resume}
	if s.resume.IsZero() {
		opts.Tail = m.tail
	}
	first := len(m.rawLines)
	m.appendOne(msg.pod, noteDropped, lineNotice)
	next, req := m.streams.open(msg.pod, s, opts, s.resume)
	m.streams = next
	m.afterNotices(first)
	return m, attachCmd(m.sess, []attachReq{req})
}

func (m Model) onRecheck(msg recheckMsg) (views.View, tea.Cmd) {
	s, ok := m.streams.pods[msg.pod]
	if !m.sess.tok.Accept(msg.tok) || !ok || s.phase != phaseWaiting || s.attempt != msg.attempt {
		return m, nil
	}
	m.streams = m.streams.recheck(msg.pod)
	return m.armExplain()
}
