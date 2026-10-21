// Package keys centralises the global keymap. Per-view overrides compose
// over the defaults defined here.
package keys

import "charm.land/bubbles/v2/key"

// Map is the set of global key bindings k4s exposes.
type Map struct {
	Quit      key.Binding
	ForceQuit key.Binding
	Help      key.Binding
	Command   key.Binding
	Back      key.Binding
	Copy      key.Binding
	YAML      key.Binding
}

// Default returns the baseline keymap.
//
// q is "go home, then quit" — the first press takes you back to the
// dashboard, the second press (now on the dashboard) exits. ctrl+c is the
// always-quit escape hatch and works even inside the command bar. y copies
// the footer's kubectl command and o opens the YAML of the object the view
// shows or has under its cursor; in a text prompt both are just letters.
func Default() Map {
	return Map{
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "home / quit"),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy"),
		),
		YAML: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "yaml"),
		),
	}
}
