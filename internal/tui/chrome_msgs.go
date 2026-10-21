package tui

import tea "charm.land/bubbletea/v2"

// onChromeMsg handles the replies to what the chrome itself asked for;
// ok is false for any other message.
func (m Model) onChromeMsg(msg tea.Msg) (next Model, ok bool) {
	switch msg := msg.(type) {
	case versionMsg:
		return m.onVersion(msg), true
	}
	return m, false
}
