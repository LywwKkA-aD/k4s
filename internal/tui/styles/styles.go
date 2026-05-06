// Package styles owns every lipgloss style used by the TUI so that visual
// changes are localised and themeable in one place.
package styles

import "github.com/charmbracelet/lipgloss"

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
)
