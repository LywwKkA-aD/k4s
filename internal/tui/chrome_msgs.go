package tui

import tea "charm.land/bubbletea/v2"

// onChromeMsg handles the replies to what the chrome itself asked for: the
// header's server version and the terminal's background. ok is false for
// any other message.
func (m Model) onChromeMsg(msg tea.Msg) (next Model, ok bool) {
	switch msg := msg.(type) {
	case versionMsg:
		return m.onVersion(msg), true
	case tea.BackgroundColorMsg:
		return m.onBackground(msg), true
	case holdDoneMsg:
		m.holdFrame = false
		return m, true
	}
	return m, false
}
