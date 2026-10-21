package status

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// SetCommand is Set for a message about cmd, such as the kubectl debug
// command after a missing shell: while the message shows, the footer's
// command row shows cmd instead of the view's and y copies it. A command
// too long for the status row stays whole there.
func (l Line) SetCommand(level Level, text string, cmd kubectl.Cmd, now time.Time) (Line, tea.Cmd) {
	next, expire := l.Set(level, text, now)
	next.cmd = cmd
	return next, expire
}

// Command returns the command the message showing at now is about, if it
// has one.
func (l Line) Command(now time.Time) (kubectl.Cmd, bool) {
	if l.cmd.IsZero() || !l.showing(now) {
		return kubectl.Cmd{}, false
	}
	return l.cmd, true
}

// CommandAtKey is Command as it stood just before the last key press. The
// root records a key before it acts on it, so the key that dismisses an
// error would otherwise copy the view's command instead of the one on
// screen when it was pressed.
func (l Line) CommandAtKey(now time.Time) (kubectl.Cmd, bool) {
	before := l
	before.keys = max(l.keys-1, 0)
	return before.Command(now)
}
