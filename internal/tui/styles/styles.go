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

	Footer = lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(colorMuted)

	KubectlHint = lipgloss.NewStyle().
			Foreground(colorAccent).
			Italic(true)
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
