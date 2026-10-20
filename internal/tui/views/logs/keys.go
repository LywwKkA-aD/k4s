package logs

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func (m Model) onKey(msg tea.KeyPressMsg) (views.View, tea.Cmd) {
	if m.searchOpen {
		return m.onSearchKey(msg)
	}
	if next, cmd, handled := m.handleViewKey(msg); handled {
		return next, cmd
	}
	prevBottom := m.viewport.AtBottom()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	nowBottom := m.viewport.AtBottom()
	if nowBottom != prevBottom {
		m.autoFollow = nowBottom
	}
	return m, cmd
}

func (m Model) handleViewKey(msg tea.KeyPressMsg) (views.View, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.followKey):
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.viewport.GotoBottom()
		}
		return m, nil, true
	case key.Matches(msg, m.tagsKey):
		// No-op for single-pod views — the prefix is suppressed there
		// regardless of this flag, so flipping it is just visual noise.
		if len(m.pods) > 1 {
			m.showPodNames = !m.showPodNames
			m.rebuildRendered()
			m.syncViewport()
		}
		return m, nil, true
	case key.Matches(msg, m.wrapKey):
		m.toggleWrap()
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, m.hscrollLeftKey):
		m.hscroll(-1)
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, m.hscrollRightKey):
		m.hscroll(+1)
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, m.searchOpenKey):
		m.searchOpen = true
		m.searchInput.Reset()
		m.searchInput.SetValue(m.searchQuery)
		m.searchInput.Focus()
		m.layoutViewport()
		return m, textinput.Blink, true
	case key.Matches(msg, m.nextMatchKey):
		m.gotoMatch(+1)
		return m, nil, true
	case key.Matches(msg, m.prevMatchKey):
		m.gotoMatch(-1)
		return m, nil, true
	case key.Matches(msg, m.clearKey):
		// Two-step clear: while a search is active (prompt open or query
		// set), 'c' cancels the search first; only when there is no search
		// does it actually wipe the buffer. Clearing the buffer must NEVER
		// touch autoFollow — the user's paused state stays paused.
		if m.searchOpen || m.searchQuery != "" {
			m.cancelSearch()
			m.syncViewport()
			return m, nil, true
		}
		m.clearLogs()
		return m, nil, true
	}
	return m, nil, false
}
