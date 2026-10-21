package logs

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onLogEventsBatch applies every event in the batch with a single render
// at the end: one format per new line and one viewport refresh per drain
// tick, however many lines a --tail=5000 flood brings. A batch from a
// session the view has since replaced is dropped.
func (m Model) onLogEventsBatch(msg logEventsBatch) (views.View, tea.Cmd) {
	if msg.gen != m.sess.gen {
		return m, nil
	}
	first := len(m.rawLines)
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
	m.matches = append(m.matches, matchesFrom(m.search, m.rawLines, first)...)
	m.settle(first)
	if msg.done {
		// Channel was observed closed mid-drain. No further reads needed.
		return m, nil
	}
	return m, waitOrDrainCmd(m.sess)
}

// settle trims the backlog to views.MaxLogLines and lays it out again. A
// paused view keeps its first visible line: it moves up by the rows the
// trim took, counted as they were laid out (first is how many lines
// were laid out then). A view that was looking at a dropped line lands on
// the oldest line left.
func (m *Model) settle(first int) {
	y, laidOut := m.viewport.YOffset(), m.rowStart
	drop := m.trim()
	m.syncViewport()
	if m.autoFollow || drop == 0 {
		return
	}
	if drop >= first || drop >= len(laidOut) {
		m.viewport.SetYOffset(0)
		return
	}
	m.viewport.SetYOffset(max(y-laidOut[drop], 0))
}

func (m *Model) clearLogs() {
	m.rawLines = nil
	m.rendered = nil
	m.matches = nil
	m.matchIdx = 0
	m.syncViewport()
}

// appendOne adds a single event to both rawLines (the truth) and rendered
// (the display cache). Format runs once at append time; later refreshes
// just join the cache.
func (m *Model) appendOne(pod, text string, kind lineKind) {
	ln := logLine{pod: pod, text: text, kind: kind}
	m.rawLines = append(m.rawLines, ln)
	m.rendered = append(m.rendered, formatRawLine(ln, len(m.pods) > 1, m.showPodNames, m.podColors[ln.pod]))
}

// trim drops the oldest lines beyond views.MaxLogLines and returns how
// many went. Matches move with their lines: the ones on dropped lines go,
// and the current match stays on its line, or falls to the first match
// left when its own line went. The kept lines are copied, so no earlier
// copy of the model shares their backing array.
func (m *Model) trim() int {
	drop := len(m.rawLines) - views.MaxLogLines
	if drop <= 0 {
		return 0
	}
	m.rawLines = append(make([]logLine, 0, views.MaxLogLines+drainBatchCap), m.rawLines[drop:]...)
	m.rendered = append(make([]string, 0, views.MaxLogLines+drainBatchCap), m.rendered[drop:]...)
	m.matches, m.matchIdx = shiftMatches(m.matches, m.matchIdx, drop)
	return drop
}

// shiftMatches renumbers matches after the first drop lines went.
func shiftMatches(matches []match, current, drop int) ([]match, int) {
	gone := sort.Search(len(matches), func(i int) bool { return matches[i].line >= drop })
	if gone == 0 {
		out := make([]match, len(matches))
		for i, mt := range matches {
			out[i] = match{line: mt.line - drop, span: mt.span}
		}
		return out, current
	}
	out := make([]match, 0, len(matches)-gone)
	for _, mt := range matches[gone:] {
		out = append(out, match{line: mt.line - drop, span: mt.span})
	}
	return out, max(current-gone, 0)
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
