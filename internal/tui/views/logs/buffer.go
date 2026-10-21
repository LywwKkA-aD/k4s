package logs

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onLogEventsBatch applies every event in the batch with a single render
// at the end: one format per new line and one viewport refresh per drain
// tick, however many lines a --tail=5000 flood brings. A batch from a
// session the view has since replaced is dropped. The stamp of each pod's
// last line is its resume point; the end of a stream is explained by the
// next check, armed here.
func (m Model) onLogEventsBatch(msg logEventsBatch) (views.View, tea.Cmd) {
	if !m.sess.tok.Accept(msg.tok) {
		return m, nil
	}
	first := len(m.rawLines)
	last := map[string]stamp{}
	cmds := []tea.Cmd{waitOrDrainCmd(m.sess)}
	explain := false
	for _, ev := range msg.events {
		if !ev.Done {
			m.appendOne(ev.Pod, ev.Line, lineLog)
			if !ev.TS.IsZero() {
				last[ev.Pod] = stamp{attempt: ev.Attempt, ts: ev.TS}
			}
			continue
		}
		if st, ok := last[ev.Pod]; ok {
			m.streams = m.streams.saw(map[string]stamp{ev.Pod: st})
			delete(last, ev.Pod)
		}
		said, ended := m.onEnd(ev)
		cmds = append(cmds, said)
		explain = explain || ended
	}
	m.streams = m.streams.saw(last)
	m.matches = append(m.matches, matchesFrom(m.search, m.rawLines, first)...)
	m.settle(first)
	if explain {
		var arm tea.Cmd
		m, arm = m.armExplain()
		cmds = append(cmds, arm)
	}
	return m, tea.Batch(cmds...)
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
	m.cells = nil
	m.bytes = 0
	m.matches = nil
	m.matchIdx = 0
	m.syncViewport()
}

// appendOne adds a single event to both rawLines (the truth) and rendered
// (the display cache). Format runs once at append time; later refreshes
// just join the cache.
func (m *Model) appendOne(pod, text string, kind lineKind) {
	ln := logLine{pod: pod, text: text, kind: kind}
	m.bytes += len(text)
	m.rawLines = append(m.rawLines, ln)
	m.rendered = append(m.rendered, formatRawLine(ln, m.multi, m.showPodNames, m.podColors[ln.pod]))
}

// trim drops the oldest lines beyond views.MaxLogLines, or beyond
// maxBacklogBytes of text, and returns how many went; the newest line
// always stays. Matches move with their lines: the ones on dropped lines
// go, and the current match stays on its line, or falls to the first
// match left when its own line went. The kept lines are copied, so no
// earlier copy of the model shares their backing array.
func (m *Model) trim() int {
	drop := max(len(m.rawLines)-views.MaxLogLines, 0)
	bytes := m.bytes
	for _, ln := range m.rawLines[:drop] {
		bytes -= len(ln.text)
	}
	for drop < len(m.rawLines)-1 && bytes > maxBacklogBytes {
		bytes -= len(m.rawLines[drop].text)
		drop++
	}
	if drop == 0 {
		return 0
	}
	keep := len(m.rawLines) - drop + drainBatchCap
	m.rawLines = append(make([]logLine, 0, keep), m.rawLines[drop:]...)
	m.rendered = append(make([]string, 0, keep), m.rendered[drop:]...)
	m.cells = m.cells[min(drop, len(m.cells)):]
	m.bytes = bytes
	if len(m.matches) > 0 && m.matches[m.matchIdx].line < drop {
		m.repaint()
	}
	m.matches, m.matchIdx = shiftMatches(m.matches, m.matchIdx, drop)
	return drop
}

// shiftMatches renumbers matches after the first drop lines went.
func shiftMatches(matches []match, current, drop int) ([]match, int) {
	if len(matches) == 0 {
		return nil, 0
	}
	gone := sort.Search(len(matches), func(i int) bool { return matches[i].line >= drop })
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
	rendered := make([]string, len(m.rawLines))
	for i, ln := range m.rawLines {
		rendered[i] = formatRawLine(ln, m.multi, m.showPodNames, m.podColors[ln.pod])
	}
	m.rendered = rendered
	m.cells = nil
}
