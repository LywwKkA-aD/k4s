package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// k4sLogo is the ASCII art wordmark for k4s (3 lines)
var k4sLogo = []string{
	"█▄▀ █ █ ▄▀▀",
	"█▀▄ ▀▀█ ▀▀▄",
	"▀ ▀   ▀ ▀▀▀",
}

// renderLogo renders the K4S logo block with diagonal stripe accents.
// Returns a block: stripe line + logo lines + stripe line.
func renderLogo(width int) string {
	if width < 4 {
		return ""
	}

	logoStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary)

	stripeStyle := lipgloss.NewStyle().
		Foreground(colorAccent)

	// Build stripe line
	stripe := stripeStyle.Render(strings.Repeat("╱", width))

	// Center logo lines
	var sb strings.Builder
	sb.WriteString(stripe)
	sb.WriteString("\n")

	for _, line := range k4sLogo {
		// Pad/center the logo line within the sidebar width
		padLeft := (width - len([]rune(line))) / 2
		if padLeft < 0 {
			padLeft = 0
		}
		padded := strings.Repeat(" ", padLeft) + line
		sb.WriteString(logoStyle.Render(padded))
		sb.WriteString("\n")
	}

	sb.WriteString(stripe)

	return sb.String()
}
