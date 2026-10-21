// Package overlay holds what the root model draws over the active view: the
// command bar, the tail, container and port prompts, the restore confirm and
// the help popup. The root keeps at most one open, and while it is open the
// overlay gets every key and paste.
package overlay

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// Overlay is a popup or prompt over the active view.
type Overlay interface {
	// Update handles a key, a paste or a cursor blink and ignores anything
	// else. A nil Overlay means it closed; what the user chose comes back
	// as a message from the returned Cmd.
	Update(msg tea.Msg) (Overlay, tea.Cmd)

	// View renders the overlay's content in at most width columns: the
	// root adds the popup card around it, or puts it in the footer.
	View(width int) string
}

// Placement is where the root draws an overlay.
type Placement int

const (
	// Popup is a bordered card centred over the body.
	Popup Placement = iota
	// Footer takes the footer's first row, above the kubectl command.
	Footer
)

// Placer is implemented by overlays that are not popups.
type Placer interface {
	Placement() Placement
}

// PlacementOf reports where o is drawn.
func PlacementOf(o Overlay) Placement {
	if p, ok := o.(Placer); ok {
		return p.Placement()
	}
	return Popup
}

// Passer is implemented by overlays that close on input they have no use
// for and leave it to the root, as if they had not been open.
type Passer interface {
	PassesOn(msg tea.Msg) bool
}

// Passes reports whether the root should close o and handle msg itself.
func Passes(o Overlay, msg tea.Msg) bool {
	p, ok := o.(Passer)
	return ok && p.PassesOn(msg)
}

// Grace is how long a confirm ignores keys after it opens, so a key typed
// ahead for the view underneath cannot answer a question nobody has read.
const Grace = 400 * time.Millisecond

// send is a Cmd that delivers msg.
func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// Commander is implemented by overlays that preview a kubectl command: the
// tail prompt shows the logs it would open, the container picker the logs
// or exec its pick leads to. The root shows it in the footer in place of
// the view's command, and y copies it. Like a view's, it carries neither
// --context nor --kubeconfig; the root adds them.
type Commander interface {
	Command() kubectl.Cmd
}

// CommandOf returns o's command; ok is false when o has none, and the
// root then shows the view's.
func CommandOf(o Overlay) (cmd kubectl.Cmd, ok bool) {
	c, ok := o.(Commander)
	if !ok {
		return kubectl.Cmd{}, false
	}
	return c.Command(), true
}

// Typer is implemented by overlays with a text input. While one is open
// every printable key is text, y included; any other overlay leaves y to
// the root, which copies the command instead.
type Typer interface {
	TakesText() bool
}

// TakesText reports whether o has a text input.
func TakesText(o Overlay) bool {
	t, ok := o.(Typer)
	return ok && t.TakesText()
}
