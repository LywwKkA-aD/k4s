// Package keys centralises the global keymap. Per-view overrides compose
// over the defaults defined here.
package keys

import "github.com/charmbracelet/bubbles/key"

// Map is the set of global key bindings k4s exposes.
type Map struct {
	Quit    key.Binding
	Help    key.Binding
	Command key.Binding
	Back    key.Binding
}

// Default returns the baseline keymap.
func Default() Map {
	return Map{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
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
	}
}
