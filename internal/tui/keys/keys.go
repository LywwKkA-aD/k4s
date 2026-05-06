// Package keys centralises the global keymap. Per-view overrides will compose
// over the defaults defined here as new screens are added.
package keys

import "github.com/charmbracelet/bubbles/key"

// Map is the set of global key bindings k4s exposes.
type Map struct {
	Quit key.Binding
	Help key.Binding
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
	}
}
