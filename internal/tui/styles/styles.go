// Package styles owns every lipgloss style used by the TUI so that visual
// changes are localised and themeable in one place.
package styles

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccent  = lipgloss.Color("#7D56F4")
	colorMuted   = lipgloss.Color("#626262")
	colorSuccess = lipgloss.Color("#04B575")
	colorWarn    = lipgloss.Color("#F2A65A")
)

// Foreground / inline styles used across views.
var (
	Title    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	Subtitle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	Hint     = lipgloss.NewStyle().Foreground(colorMuted).Faint(true)
	OK       = lipgloss.NewStyle().Foreground(colorSuccess)
	Warn     = lipgloss.NewStyle().Foreground(colorWarn)

	Stat = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMuted)

	Header = lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(colorMuted)

	// Footer reserves an extra line at the bottom (PaddingBottom) so the
	// chrome does not sit directly on the terminal edge — handy for tmux
	// status bars and so the kubectl hint never hugs the system prompt.
	Footer = lipgloss.NewStyle().
		Padding(0, 1, 1, 1).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(colorMuted)

	KubectlHint = lipgloss.NewStyle().
			Foreground(colorAccent).
			Italic(true)

	// Highlight is used for search matches in the logs view.
	Highlight = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFD166")).
			Bold(true)

	// PopupBox is the bordered card used by interactive prompts (tail
	// lines, container picker) so they pop over the body instead of
	// hiding among the footer hints.
	PopupBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 3)

	// PopupTitle is the bold accent header inside a popup.
	PopupTitle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
)

// Table returns the table styles used across views.
func Table() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		Foreground(colorAccent).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		BorderBottom(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(colorAccent).
		Bold(true)
	return s
}
