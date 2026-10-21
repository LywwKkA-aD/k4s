package logs

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// session is one run of the streams behind the view. Its events channel
// closes once every stream has ended. Batches carry the gen they were
// read under, so the reads a replaced session left in flight never land
// in its successor.
type session struct {
	gen    int
	events chan logEvent
	cancel context.CancelFunc
}

// stop cancels the session's streams; the zero session has none.
func (s session) stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// streamSpec is what a session streams, and where a panic in one of its
// goroutines is logged.
type streamSpec struct {
	src       streamer
	namespace string
	pods      []string
	tail      int64 // per pod
	container string
	crash     crashLog
}

func (m Model) spec() streamSpec {
	return streamSpec{
		src: m.src, namespace: m.namespace, pods: m.pods, tail: m.tail, container: m.container, crash: m.crash,
	}
}

// options are the log options of one pod's current log.
func (s streamSpec) options() k8s.LogOptions {
	return k8s.LogOptions{Container: s.container, Tail: s.tail}
}

// startSession starts the streams of spec: one pod is read as it comes,
// several are merged (see runMergedStreams). Without a client or pods the
// events channel is closed at once.
func startSession(gen int, spec streamSpec) session {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan logEvent, eventBufferSize)
	switch {
	case spec.src == nil || len(spec.pods) == 0:
		close(events)
	case len(spec.pods) == 1:
		pod := spec.pods[0]
		go func() {
			defer close(events)
			defer spec.crash.guard("the log stream of pod "+pod, func(err error) {
				send(ctx, events, logEvent{Pod: pod, Err: err, Done: true})
			})
			streamOnePod(ctx, spec.src, spec.namespace, pod, spec.options(), events)
		}()
	default:
		go runMergedStreams(ctx, spec, events)
	}
	return session{gen: gen, events: events, cancel: cancel}
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

// waitOrDrainCmd blocks on the session's first event (so the bubbletea
// loop stays quiet while no logs flow) then drains everything else the
// channel has queued in one go. One Update tick rebuilds the viewport once
// instead of 5000 times when --tail=5000 arrives in a flood.
func waitOrDrainCmd(s session) tea.Cmd {
	ch, gen := s.events, s.gen
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return streamsClosedMsg{gen: gen}
		}
		rest, done := drainEvents(ch, drainBatchCap-1)
		batch := make([]logEvent, 0, 1+len(rest))
		batch = append(batch, first)
		batch = append(batch, rest...)
		return logEventsBatch{gen: gen, events: batch, done: done}
	}
}
