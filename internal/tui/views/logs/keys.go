package logs

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// keyMap is the view's own bindings; the root owns q, :, ?, Esc and y.
type keyMap struct {
	follow     key.Binding
	tags       key.Binding
	wrap       key.Binding
	left       key.Binding
	right      key.Binding
	searchOpen key.Binding
	next       key.Binding
	prev       key.Binding
	clear      key.Binding
	scroll     key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		follow:     key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),
		tags:       key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag pods")),
		wrap:       key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "wrap")),
		left:       key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "scroll left")),
		right:      key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "scroll right")),
		searchOpen: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		next:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		prev:       key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		clear:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
		scroll: key.NewBinding(
			key.WithKeys("up", "down", "pgup", "pgdown"),
			key.WithHelp("↑↓/pgup/pgdn", "scroll"),
		),
	}
}

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
	k := m.keys
	switch {
	case key.Matches(msg, k.follow):
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.viewport.GotoBottom()
		}
	case key.Matches(msg, k.tags):
		// No-op for single-pod views — the prefix is suppressed there
		// regardless of this flag, so flipping it is just visual noise.
		if len(m.pods) > 1 {
			m.showPodNames = !m.showPodNames
			m.rebuildRendered()
			m.syncViewport()
		}
	case key.Matches(msg, k.wrap):
		m.toggleWrap()
		m.syncViewport()
	case key.Matches(msg, k.left):
		m.hscroll(-1)
		m.syncViewport()
	case key.Matches(msg, k.right):
		m.hscroll(+1)
		m.syncViewport()
	case key.Matches(msg, k.searchOpen):
		m.openSearch()
		return m, textinput.Blink, true
	case key.Matches(msg, k.next):
		m.gotoMatch(+1)
	case key.Matches(msg, k.prev):
		m.gotoMatch(-1)
	case key.Matches(msg, k.clear):
		m.clearOrCancel()
	default:
		return m, nil, false
	}
	return m, nil, true
}

// clearOrCancel is the two-step clear: while a search is active (prompt
// open or query set), 'c' cancels the search first; only when there is
// no search does it wipe the buffer. Neither touches autoFollow — the
// user's paused state stays paused.
func (m *Model) clearOrCancel() {
	if m.searchOpen || m.search.text != "" {
		m.cancelSearch()
		m.syncViewport()
		return
	}
	m.clearLogs()
}
