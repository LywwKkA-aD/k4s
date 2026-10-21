package overlay

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// login is a Confirm that previews the command it would run, so the footer
// shows it and y copies it.
type login struct {
	Overlay
	run kubectl.Cmd
}

// NewLogin asks whether to log in to context in a terminal, for a
// credential plugin that cannot prompt while k4s owns it. run is the
// command that logs in (`kubectl version`; the root adds --context); Yes
// is sent on Enter, No on n, q or Esc.
func NewLogin(context string, run kubectl.Cmd, yes, no tea.Msg, now func() time.Time) Overlay {
	return login{
		Overlay: NewConfirm(Confirm{
			Title: "log in",
			Question: "Context " + context + " needs a login: its credential plugin cannot\n" +
				"prompt while k4s owns the terminal. log in in a terminal?",
			Hint: "Enter runs the kubectl command below · n / Esc skip",
			Yes:  yes,
			No:   no,
		}, now),
		run: run,
	}
}

func (l login) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	next, cmd := l.Overlay.Update(msg)
	if next == nil {
		return nil, cmd
	}
	l.Overlay = next
	return l, cmd
}

// Command implements Commander.
func (l login) Command() kubectl.Cmd { return l.run }
