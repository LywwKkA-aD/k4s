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

// status is the toast as the status line shows it, which leads a message
// with its level's glyph (styles.GlyphOK for ok, GlyphWarn for warn), so
// the row shows one glyph, the change's severity's: a recovery goes as
// ok and a change to waiting as warn, without a glyph of their own. A
// failing change has no level that fits: warn would show △, and an error
// stays up until a key. It goes as info, which has no glyph, with its own
// ▲, as minor and info changes go with theirs.
func (t toast) status() (status.Level, string) {
	text := t.c.object() + " → " + clean(t.c.to)
	if t.more > 0 {
		text += fmt.Sprintf(" · +%d", t.more)
	}
	switch t.c.sev {
	case dash.SevOK:
		return status.OK, text
	case dash.SevWaiting:
		return status.Warn, text
	}
	return status.Info, markGlyphs[markOf(t.c.sev)] + " " + text
}

// raise shows the first of changes: in the banner, or in the status line
// outside the wide layout.
func (m Model) raise(changes []change) (Model, tea.Cmd) {
	if len(changes) == 0 {
		return m, nil
	}
	t := toast{c: changes[0], more: len(changes) - 1}
	if m.lay.kind != kindWide {
		return m, statusCmd(t.status())
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
