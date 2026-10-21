package dashboard

import "charm.land/bubbles/v2/key"

// keyMap is the dashboard's own keys. q, :, ?, Esc, y and ctrl+c are the
// root's. h is deliberately unbound and l moves no focus: l means logs
// everywhere else, and here too.
type keyMap struct {
	next, prev  key.Binding
	jump        key.Binding
	left, right key.Binding
	up, down    key.Binding
	enter       key.Binding
	logs        key.Binding
	sort        key.Binding
	refresh     key.Binding
	watch       key.Binding

	// panel, side and row are the footer hints for the keys above.
	panel, side, row key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		next:    key.NewBinding(key.WithKeys("tab")),
		prev:    key.NewBinding(key.WithKeys("shift+tab")),
		jump:    key.NewBinding(key.WithKeys("0", "1", "2", "3", "4", "5", "6", "7")),
		left:    key.NewBinding(key.WithKeys("left")),
		right:   key.NewBinding(key.WithKeys("right")),
		up:      key.NewBinding(key.WithKeys("up", "k")),
		down:    key.NewBinding(key.WithKeys("down", "j")),
		enter:   key.NewBinding(key.WithKeys("enter")),
		logs:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		sort:    key.NewBinding(key.WithKeys("s")),
		refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		watch:   key.NewBinding(key.WithKeys("w")),
		panel:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/0-7", "panel")),
		side:    key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←→", "neighbour")),
		row:     key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "row")),
	}
}
