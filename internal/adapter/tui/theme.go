package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// K4sHuhTheme returns a custom Huh theme using our application colors.
// We strip the default left border since our dialogs provide their own outer chrome.
func K4sHuhTheme() *huh.Theme {
	t := huh.ThemeBase()

	white := lipgloss.Color("#FFFFFF")
	muted := colorMuted
	primary := colorPrimary
	errColor := colorError

	// Remove the default thick left border — our dialog wrapper adds its own border
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1)

	// Focused field styles
	t.Focused.Title = lipgloss.NewStyle().Foreground(primary).Bold(true)
	t.Focused.Description = lipgloss.NewStyle().Foreground(muted).Italic(true)
	t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(errColor)
	t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(errColor)

	// Select styles
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(primary)
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(white).Bold(true)
	t.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(muted)

	// Confirm button styles
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Background(primary).Foreground(white).Bold(true).Padding(0, 2)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Background(lipgloss.Color("#444444")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 2)

	// Text input styles
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(primary)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(primary)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(white)

	// Blurred field styles (mirrors focused but dimmer)
	t.Blurred.Title = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.Description = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.Placeholder = lipgloss.NewStyle().Foreground(muted)
	t.Blurred.TextInput.Text = lipgloss.NewStyle().Foreground(muted)

	// Blurred buttons match focused (single-field forms always focused)
	t.Blurred.FocusedButton = t.Focused.FocusedButton
	t.Blurred.BlurredButton = t.Focused.BlurredButton

	// Select styles for blurred state
	t.Blurred.SelectSelector = lipgloss.NewStyle().Foreground(colorSubtle)
	t.Blurred.SelectedOption = lipgloss.NewStyle().Foreground(colorMuted)
	t.Blurred.UnselectedOption = lipgloss.NewStyle().Foreground(colorSubtle)

	return t
}
