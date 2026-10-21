package dashboard

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// A toast names the change a poll brought (§6): one at a time, the most
// severe, with +N for the rest of that poll's changes. The wide layout
// shows it in the banner's toast lane and dismisses it with one 5s tick;
// the narrower ones hand it to the root's status line, which times it out
// itself. The first snapshot raises none: everything would be news.

// toastFor is how long a toast stays in the banner.
const toastFor = 5 * time.Second

// toastDoneMsg dismisses the toast that tok raised.
type toastDoneMsg struct{ tok async.Token }

// toast is the change on show and how many others came with it.
type toast struct {
	c    change
	more int
	tok  async.Token
}

// text is the toast as words: "▲ pod shop/api → CrashLoopBackOff".
func (t toast) text() string {
	return markGlyphs[markOf(t.c.sev)] + " " + t.c.object() + " → " + clean(t.c.to)
}

// raise shows the first of changes: in the banner, or in the status line
// outside the wide layout.
func (m Model) raise(changes []change) (Model, tea.Cmd) {
	if len(changes) == 0 {
		return m, nil
	}
	t := toast{c: changes[0], more: len(changes) - 1}
	if m.lay.kind != kindWide {
		text := t.text()
		if t.more > 0 {
			text += fmt.Sprintf(" · +%d", t.more)
		}
		return m, statusCmd(toastLevel(t.c.sev), text)
	}
	m.mo.mint = m.mo.mint.Next()
	t.tok = m.mo.mint
	m.toast = t
	tok := t.tok
	return m, m.frames.Tick(toastFor, func(time.Time) tea.Msg { return toastDoneMsg{tok: tok} })
}

func (m Model) onToastDone(msg toastDoneMsg) (views.View, tea.Cmd) {
	if m.toast.tok.Accept(msg.tok) {
		m.toast = toast{}
	}
	return m, nil
}

// toastLevel is the status level of a toast: a change for the worse
// warns, a recovery is ok. Never error: that would stay until a key.
func toastLevel(s dash.Severity) status.Level {
	switch {
	case s >= dash.SevWaiting:
		return status.Warn
	case s == dash.SevOK:
		return status.OK
	}
	return status.Info
}

// toastNote is the toast lane on the banner's second row, "" when there
// is no toast.
func (f frame) toastNote() string {
	t := f.toast
	if t.tok == (async.Token{}) {
		return ""
	}
	text := f.paint(markOf(t.c.sev)) + " " + t.c.object() + " → " + clean(t.c.to)
	if t.more > 0 {
		text += f.muted(fmt.Sprintf("  +%d", t.more))
	}
	return text + " "
}
