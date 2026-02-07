package tui

import "github.com/charmbracelet/lipgloss"

// Theme colors — Crush-inspired palette
var (
	// Base palette
	colorPrimary   = lipgloss.Color("#7D56F4")
	colorAccent    = lipgloss.Color("#AD58F7")
	colorSecondary = lipgloss.Color("#5A5A5A")
	colorSuccess   = lipgloss.Color("#73D216")
	colorWarning   = lipgloss.Color("#F5A623")
	colorError     = lipgloss.Color("#FF5F56")

	// Text hierarchy
	colorText   = lipgloss.Color("#E0E0E0")
	colorMuted  = lipgloss.Color("#626262")
	colorSubtle = lipgloss.Color("#444444")
	colorDim    = lipgloss.Color("#333333")

	// Backgrounds
	colorBgDark      = lipgloss.Color("#1a1a2e")
	colorBgHighlight = lipgloss.Color("#2a2550")

	// Borders
	colorBorder = lipgloss.Color("#333333")
)

// Styles defines the application styling
type Styles struct {
	App       lipgloss.Style
	Header    lipgloss.Style // fallback for narrow terminals
	Title     lipgloss.Style
	Subtitle  lipgloss.Style
	Content   lipgloss.Style
	Footer    lipgloss.Style
	Help      lipgloss.Style
	StatusBar lipgloss.Style
}

// DefaultStyles returns the default theme styles
func DefaultStyles() Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(1, 2),

		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colorDim).
			MarginBottom(1),

		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),

		Subtitle: lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true),

		Content: lipgloss.NewStyle().
			Padding(1, 0),

		Footer: lipgloss.NewStyle().
			Foreground(colorMuted).
			PaddingTop(0),

		Help: lipgloss.NewStyle().
			Foreground(colorMuted),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(colorPrimary).
			Padding(0, 1),
	}
}
