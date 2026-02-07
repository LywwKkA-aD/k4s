package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpScreen displays keyboard shortcuts
type HelpScreen struct {
	visible bool
	width   int
	height  int
}

// NewHelpScreen creates a new help screen
func NewHelpScreen() HelpScreen {
	return HelpScreen{}
}

// Show displays the help screen
func (h *HelpScreen) Show() {
	h.visible = true
}

// Hide hides the help screen
func (h *HelpScreen) Hide() {
	h.visible = false
}

// Toggle toggles the help screen visibility
func (h *HelpScreen) Toggle() {
	h.visible = !h.visible
}

// IsVisible returns true if the help screen is visible
func (h *HelpScreen) IsVisible() bool {
	return h.visible
}

// SetSize sets the help screen dimensions
func (h *HelpScreen) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// View renders the help screen
func (h *HelpScreen) View() string {
	if !h.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary).
		MarginBottom(1)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorMuted)

	keyStyle := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Width(10)

	descStyle := lipgloss.NewStyle().
		Foreground(colorText)

	mutedStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)

	// Column 1: Global + Navigation
	var col1 strings.Builder
	col1.WriteString(sectionStyle.Render("Global"))
	col1.WriteString("\n")
	col1.WriteString(renderShortcut(keyStyle, descStyle, "?", "Help"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "q", "Quit"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "Esc", "Back"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "r", "Refresh"))
	col1.WriteString("\n")
	col1.WriteString(sectionStyle.Render("Navigation"))
	col1.WriteString("\n")
	col1.WriteString(renderShortcut(keyStyle, descStyle, "↑/↓", "Move"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "Enter", "Select"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "/", "Filter"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "1-5", "Views"))
	col1.WriteString(renderShortcut(keyStyle, descStyle, "9", "SSH"))

	// Column 2: Pod + Deployment actions
	var col2 strings.Builder
	col2.WriteString(sectionStyle.Render("Pods"))
	col2.WriteString("\n")
	col2.WriteString(renderShortcut(keyStyle, descStyle, "l", "Logs"))
	col2.WriteString(renderShortcut(keyStyle, descStyle, "d", "Delete"))
	col2.WriteString(renderShortcut(keyStyle, descStyle, "R", "Restart"))
	col2.WriteString(renderShortcut(keyStyle, descStyle, "m", "Metrics"))
	col2.WriteString("\n")
	col2.WriteString(sectionStyle.Render("Deployments"))
	col2.WriteString("\n")
	col2.WriteString(renderShortcut(keyStyle, descStyle, "s", "Scale"))
	col2.WriteString(renderShortcut(keyStyle, descStyle, "d", "Delete"))
	col2.WriteString(renderShortcut(keyStyle, descStyle, "R", "Restart"))

	// Column 3: Events + Logs viewer
	var col3 strings.Builder
	col3.WriteString(sectionStyle.Render("Events"))
	col3.WriteString("\n")
	col3.WriteString(renderShortcut(keyStyle, descStyle, "f", "Follow"))
	col3.WriteString(renderShortcut(keyStyle, descStyle, "w", "Warnings"))
	col3.WriteString(renderShortcut(keyStyle, descStyle, "k", "Kind"))
	col3.WriteString("\n")
	col3.WriteString(sectionStyle.Render("Logs"))
	col3.WriteString("\n")
	col3.WriteString(renderShortcut(keyStyle, descStyle, "f", "Follow"))
	col3.WriteString(renderShortcut(keyStyle, descStyle, "t", "Timestamps"))
	col3.WriteString(renderShortcut(keyStyle, descStyle, "c", "Container"))
	col3.WriteString(renderShortcut(keyStyle, descStyle, "g/G", "Top/Bottom"))

	// Column style
	colStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Width(28)

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		colStyle.Render(col1.String()),
		colStyle.Render(col2.String()),
		colStyle.Render(col3.String()),
	)

	// Build final content
	var content strings.Builder
	content.WriteString(titleStyle.Render("k4s Keyboard Shortcuts"))
	content.WriteString("\n")
	content.WriteString(columns)
	content.WriteString("\n")
	content.WriteString(mutedStyle.Render("Press ? or Esc to close"))

	// Create the box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2)

	return boxStyle.Render(content.String())
}

func renderShortcut(keyStyle, descStyle lipgloss.Style, key, desc string) string {
	return keyStyle.Render(key) + descStyle.Render(desc) + "\n"
}
