package tui

import tea "charm.land/bubbletea/v2"

// onChromeMsg handles the replies to what the chrome itself asked for: the
// terminal's background and the end of the first frame's hold. ok is false
// for any other message. The header's server version applies itself
// (header_version.go), since a failed reply arms a retry.
func (m Model) onChromeMsg(msg tea.Msg) (next Model, ok bool) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		return m.onBackground(msg), true
	case holdDoneMsg:
		m.holdFrame = false
		return m, true
	}
	return m, false
}
