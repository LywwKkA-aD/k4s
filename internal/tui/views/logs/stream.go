package logs

import (
	"context"
	"errors"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// streamOnePod follows one pod's log into ch: every line made inert on the
// way in, then a closing event.
func streamOnePod(ctx context.Context, src streamer, ns, pod string, opts k8s.LogOptions, ch chan<- logEvent) {
	stream, err := src.StreamLogs(ctx, ns, pod, opts)
	if err != nil {
		send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
		return
	}
	defer func() { _ = stream.Close() }()

	lines := newLineReader(stream)
	for {
		line, err := lines.next()
		if line.ok && !send(ctx, ch, logEvent{Pod: pod, Line: line.text()}) {
			return
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() == nil {
				send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
			}
			return
		}
	}
	send(ctx, ch, logEvent{Pod: pod, Done: true})
}

func send(ctx context.Context, ch chan<- logEvent, ev logEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
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

// waitOrDrainCmd blocks on the first event (so the bubbletea loop stays
// quiet while no logs flow) then drains everything else the channel has
// queued in one go. One Update tick rebuilds the viewport once instead
// of 5000 times when --tail=5000 arrives in a flood.
func waitOrDrainCmd(ch <-chan logEvent) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return streamsClosedMsg{}
		}
		rest, done := drainEvents(ch, drainBatchCap-1)
		batch := make([]logEvent, 0, 1+len(rest))
		batch = append(batch, first)
		batch = append(batch, rest...)
		return logEventsBatch{events: batch, done: done}
	}
}

// onLogEventsBatch applies every event in the batch with a single render
// at the end. This is the heart of the slow-tail fix: instead of N²
// formatRawLine calls and N viewport.SetContent invocations, we do one of
// each per drain tick.
func (m Model) onLogEventsBatch(msg logEventsBatch) (views.View, tea.Cmd) {
	for _, ev := range msg.events {
		switch {
		case ev.Err != nil:
			m.appendOne(ev.Pod, sanitize.Line(ev.Err.Error()), lineStreamErr)
		case ev.Done:
			m.appendOne(ev.Pod, "", lineStreamDone)
		default:
			m.appendOne(ev.Pod, ev.Line, lineLog)
		}
	}
	if m.searchQuery != "" {
		m.computeMatches()
	}
	m.syncViewport()
	if msg.done {
		// Channel was observed closed mid-drain. No further reads needed.
		return m, nil
	}
	return m, waitOrDrainCmd(m.events)
}

func (m *Model) clearLogs() {
	m.rawLines = nil
	m.rendered = nil
	m.viewport.SetContent("")
}

// appendOne adds a single event to both rawLines (the truth) and rendered
// (the display cache). Format runs once at append time; later refreshes
// just join the cache. The cap is enforced on both slices in lockstep so
// they never disagree about indices.
//
// Replaces the old appendRaw which only mutated rawLines and forced a
// full re-format on every tick.
func (m *Model) appendOne(pod, text string, kind lineKind) {
	multi := len(m.pods) > 1
	ln := logLine{pod: pod, text: text, kind: kind}
	m.rawLines = append(m.rawLines, ln)
	m.rendered = append(m.rendered, formatRawLine(ln, multi, m.showPodNames, m.podColors[ln.pod]))
	if len(m.rawLines) > maxBufferedLines {
		drop := len(m.rawLines) - maxBufferedLines
		m.rawLines = m.rawLines[drop:]
		m.rendered = m.rendered[drop:]
		// Match indices refer to the old positions; cheaper to recompute
		// on the next event than to shift them.
		m.matches = nil
		m.matchIdx = 0
	}
}

// rebuildRendered re-formats every rawLine according to the current
// display flags. Called only when a mode toggle (showPodNames) changes
// the way *every* line should look — never on the hot append path.
//
// It always allocates: Update runs on copies of Model, and writing into the
// shared backing array would re-format lines an earlier copy still renders.
func (m *Model) rebuildRendered() {
	multi := len(m.pods) > 1
	rendered := make([]string, len(m.rawLines))
	for i, ln := range m.rawLines {
		rendered[i] = formatRawLine(ln, multi, m.showPodNames, m.podColors[ln.pod])
	}
	m.rendered = rendered
}
