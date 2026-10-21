package logs

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// session is one run of the streams behind the view: a pump that hands
// their lines to the view, and the readers the view attaches to it as pods
// start, join or reconnect. Several pods are merged by line time (see
// merger); one pod's lines pass straight through. Everything stops with
// the session (Close, or the switch to a container's previous instance).
// Batches carry the session's Token, so the reads a replaced session or
// another logs view left in flight never land here.
type session struct {
	tok      async.Token
	ctx      context.Context
	cancel   context.CancelFunc
	incoming chan parsedEvent
	events   chan logEvent
	src      streamer
	ns       string
	crash    crashLog
}

// pumpMode is how a session hands its lines on.
type pumpMode int

const (
	pumpDirect pumpMode = iota
	pumpMerged
)

// attachReq opens one reader: pod's log as opts says, without the lines
// stamped at or before after, under the view's attempt number.
type attachReq struct {
	pod     string
	attempt uint64
	opts    k8s.LogOptions
	after   time.Time
}

// startSession starts a session's pump. Merged, its history phase waits
// for pods (each sending up to tail lines) before lines go out in order.
func startSession(tok async.Token, src streamer, ns string, crash crashLog, mode pumpMode, pods []string, tail int64) session {
	ctx, cancel := context.WithCancel(context.Background())
	s := newSession(ctx, cancel, tok, src, ns, crash, len(pods))
	go s.pump(mode, pods, tail)
	return s
}

// newSession is a session over ctx, whose inbox holds a few events for
// each of pods pods before a reader waits.
func newSession(ctx context.Context, cancel context.CancelFunc, tok async.Token, src streamer, ns string, crash crashLog, pods int) session {
	return session{
		tok:      tok,
		ctx:      ctx,
		cancel:   cancel,
		incoming: make(chan parsedEvent, max(pods, 1)*mergeIncomingBufferCap),
		events:   make(chan logEvent, eventBufferSize),
		src:      src,
		ns:       ns,
		crash:    crash,
	}
}

// stop ends the session's pump and readers; the zero session has none.
func (s session) stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// live reports a session that has not been stopped.
func (s session) live() bool {
	return s.ctx != nil && s.ctx.Err() == nil
}

// pump hands the readers' events on. A panic in it ends the pump with an
// internal error, and the view's streams with it.
func (s session) pump(mode pumpMode, pods []string, tail int64) {
	if mode == pumpDirect {
		defer s.crash.guard("the log stream", s.pumpFailed)
		passLoop(s.ctx, s.incoming, s.events)
		return
	}
	s.merge(newMerger(pods, tail, time.Now()))
}

// merge runs the merge loop over g.
func (s session) merge(g *merger) {
	defer s.crash.guard("the log merge", s.pumpFailed)
	mergeLoop(s.ctx, s.incoming, s.events, g, time.Now)
}

func (s session) pumpFailed(err error) {
	send(s.ctx, s.events, logEvent{Err: err, Done: true})
}

// attach opens r's reader on its own goroutine. A panic in it ends that
// pod's stream with an internal error.
func (s session) attach(r attachReq) {
	if !s.live() {
		return
	}
	go func() {
		defer s.crash.guard("the log stream of pod "+r.pod, s.readerFailed(r))
		readStream(s.ctx, s.src, s.ns, r, s.incoming)
	}()
}

// attachOpen reads r's stream, already open, on its own goroutine.
func (s session) attachOpen(r attachReq, stream io.ReadCloser) {
	go func() {
		defer s.crash.guard("the log stream of pod "+r.pod, s.readerFailed(r))
		readOpen(s.ctx, stream, r, s.incoming)
	}()
}

// readerFailed ends r's stream with err, a recovered panic.
func (s session) readerFailed(r attachReq) func(error) {
	return func(err error) {
		sendParsed(s.ctx, s.incoming, parsedEvent{pod: r.pod, attempt: r.attempt, kind: lineStreamErr, err: err})
	}
}

// attachCmd opens the readers of reqs; it answers nothing.
func attachCmd(s session, reqs []attachReq) tea.Cmd {
	if len(reqs) == 0 {
		return nil
	}
	return func() tea.Msg {
		for _, r := range reqs {
			s.attach(r)
		}
		return nil
	}
}

// drainEvents pulls every event the channel currently has ready (no
// blocking past the first read), up to max. The returned done flag is
// true iff the channel was observed closed during the drain.
//
// Pulled out as a free function so it is testable without spinning up a
// tea.Program.
func drainEvents(ch <-chan logEvent, max int) ([]logEvent, bool) {
	if max <= 0 {
		return nil, false
	}
	batch := make([]logEvent, 0, 8)
	for len(batch) < max {
		select {
		case ev, ok := <-ch:
			if !ok {
				return batch, true
			}
			batch = append(batch, ev)
		default:
			return batch, false
		}
	}
	return batch, false
}

// waitOrDrainCmd blocks on the session's next event (so the bubbletea
// loop stays quiet while no logs flow) then drains everything else the
// channel has queued in one go. One Update tick rebuilds the viewport once
// instead of 5000 times when --tail=5000 arrives in a flood. A stopped
// session answers nothing.
func waitOrDrainCmd(s session) tea.Cmd {
	if s.events == nil {
		return nil
	}
	ch, tok, done := s.events, s.tok, s.ctx.Done()
	return func() tea.Msg {
		select {
		case <-done:
			return nil
		case first := <-ch:
			rest, _ := drainEvents(ch, drainBatchCap-1)
			batch := make([]logEvent, 0, 1+len(rest))
			batch = append(batch, first)
			batch = append(batch, rest...)
			return logEventsBatch{tok: tok, events: batch}
		}
	}
}
