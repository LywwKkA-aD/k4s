package logs

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func (m *Model) cancelSearch() {
	m.searchQuery = ""
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
		m.searchQuery = strings.TrimSpace(m.searchInput.Value())
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
		m.computeMatches()
		m.matchIdx = 0
		if len(m.matches) > 0 {
			m.scrollToLine(m.matches[0])
			m.autoFollow = false
		}
		m.syncViewport()
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

// computeMatches runs the substring search against the *raw* line text so
// matches are independent of the current prefix mode.
func (m *Model) computeMatches() {
	m.matches = nil
	if m.searchQuery == "" {
		return
	}
	for i, ln := range m.rawLines {
		if strings.Contains(ln.text, m.searchQuery) {
			m.matches = append(m.matches, i)
		}
	}
	if m.matchIdx >= len(m.matches) {
		m.matchIdx = 0
	}
}

func (m *Model) gotoMatch(direction int) {
	if len(m.matches) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + direction + len(m.matches)) % len(m.matches)
	m.scrollToLine(m.matches[m.matchIdx])
	m.autoFollow = false
}

func (m *Model) scrollToLine(line int) {
	target := line - m.viewport.Height()/2
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
}

func highlightContent(content, query string) string {
	if query == "" {
		return content
	}
	return strings.ReplaceAll(content, query, styles.Highlight.Render(query))
}
