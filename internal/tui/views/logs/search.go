package logs

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func (m *Model) openSearch() {
	m.searchOpen = true
	m.searchInput.Reset()
	m.searchInput.SetValue(m.search.text)
	m.searchInput.Focus()
	m.layoutViewport()
}

func (m *Model) cancelSearch() {
	m.repaint()
	m.search = query{}
	m.matches = nil
	m.matchIdx = 0
	m.searchInput.Reset()
	if m.searchOpen {
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
	}
}

func (m Model) onSearchKey(msg tea.KeyPressMsg) (views.View, tea.Cmd) {
	switch {
	case keys.IsEsc(msg):
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
		return m, nil
	case keys.IsEnter(msg):
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
		m.search = newQuery(strings.TrimSpace(m.searchInput.Value()))
		m.computeMatches()
		m.matchIdx = 0
		if len(m.matches) == 0 {
			m.syncViewport()
			return m, nil
		}
		m.autoFollow = false
		m.focusMatch()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// onPaste types pasted text into the open search prompt. With the prompt
// closed a paste does nothing, like any key that matches no binding.
func (m Model) onPaste(msg tea.PasteMsg) (views.View, tea.Cmd) {
	if !m.searchOpen {
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// computeMatches finds every occurrence of the query in the whole backlog.
// It searches the sanitized line text, never the prefix, so the pod name
// and the colour codes around it never match.
func (m *Model) computeMatches() {
	m.repaint()
	m.matches = matchesFrom(m.search, m.rawLines, 0)
	if m.matchIdx >= len(m.matches) {
		m.matchIdx = 0
	}
}

// matchesFrom returns the occurrences of q in lines[from:], numbered by
// their index in lines.
func matchesFrom(q query, lines []logLine, from int) []match {
	if q.text == "" {
		return nil
	}
	var out []match
	for i := from; i < len(lines); i++ {
		for _, s := range q.find(lines[i].text) {
			out = append(out, match{line: i, span: s})
		}
	}
	return out
}

func (m *Model) gotoMatch(direction int) {
	if len(m.matches) == 0 {
		return
	}
	m.repaint()
	m.matchIdx = (m.matchIdx + direction + len(m.matches)) % len(m.matches)
	m.autoFollow = false
	m.focusMatch()
}

// focusMatch brings the current match into view: horizontally first when
// lines are clipped, then the row it lands on, wrapped lines counted,
// into the middle of the viewport.
func (m *Model) focusMatch() {
	mt := m.matches[m.matchIdx]
	plain, offset := m.matchOffset(mt)
	if !m.wrap {
		m.revealColumn(plain, offset, mt.end-mt.start)
	}
	m.syncViewport()
	row := mt.line
	if mt.line < len(m.rowStart) {
		row = m.rowStart[mt.line]
	}
	if m.wrap {
		row += rowWithin(plain, offset, m.viewport.Width())
	}
	m.viewport.SetYOffset(max(0, row-m.viewport.Height()/2))
}

// matchOffset returns mt's line as drawn without colour, and where the
// match starts in it: the text is the end of every drawn line.
func (m Model) matchOffset(mt match) (string, int) {
	plain := ansi.Strip(m.rendered[mt.line])
	offset := len(plain) - len(m.rawLines[mt.line].text) + mt.start
	return plain, min(max(offset, 0), len(plain))
}

// revealColumn pans a clipped view so the match starting at offset of
// plain is on screen, with some context to its left.
func (m *Model) revealColumn(plain string, offset, length int) {
	width := m.viewport.Width()
	if width <= 0 {
		return
	}
	col := ansi.StringWidth(plain[:offset])
	end := col + ansi.StringWidth(plain[offset:min(offset+length, len(plain))])
	if col >= m.xOffset && end <= m.xOffset+width {
		return
	}
	m.xOffset = min(max(col-width/4, 0), m.maxXOffset())
}
