package tui

import (
	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ContainerSelector is a component for selecting a container
type ContainerSelector struct {
	containers []string
	selected   string
	visible    bool
	width      int
	form       *huh.Form
}

// NewContainerSelector creates a new container selector
func NewContainerSelector() ContainerSelector {
	return ContainerSelector{}
}

// Show displays the container selector and returns a tea.Cmd.
func (c *ContainerSelector) Show(containers []string, currentContainer string) tea.Cmd {
	c.containers = containers
	c.visible = true
	c.selected = currentContainer

	// If no current container set, default to first
	if c.selected == "" && len(containers) > 0 {
		c.selected = containers[0]
	}

	opts := make([]huh.Option[string], len(containers))
	for i, name := range containers {
		opts[i] = huh.NewOption(name, name)
	}

	c.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select Container").
				Options(opts...).
				Value(&c.selected),
		),
	).WithTheme(K4sHuhTheme()).WithShowHelp(false)
	return c.form.Init()
}

// Hide hides the container selector
func (c *ContainerSelector) Hide() {
	c.visible = false
	c.form = nil
}

// IsVisible returns whether the selector is visible
func (c *ContainerSelector) IsVisible() bool {
	return c.visible
}

// SetWidth sets the selector width
func (c *ContainerSelector) SetWidth(width int) {
	c.width = width
}

// SelectedContainer returns the currently selected container name
func (c *ContainerSelector) SelectedContainer() string {
	return c.selected
}

// Update handles key messages for the selector
func (c *ContainerSelector) Update(msg tea.Msg) (selected bool, cancelled bool, cmd tea.Cmd) {
	if !c.visible || c.form == nil {
		return false, false, nil
	}

	// Handle esc/c for cancel
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc", "c":
			return false, true, nil
		}
	}

	model, formCmd := c.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		c.form = f
	}

	if c.form.State == huh.StateCompleted {
		return true, false, formCmd
	}
	if c.form.State == huh.StateAborted {
		return false, true, formCmd
	}

	return false, false, formCmd
}

// View renders the container selector
func (c *ContainerSelector) View() string {
	if !c.visible || c.form == nil || len(c.containers) == 0 {
		return ""
	}

	selectorWidth := 40
	if c.width > 0 && c.width < 50 {
		selectorWidth = c.width - 10
	}

	selectorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(selectorWidth)

	hintStyle := lipgloss.NewStyle().
		Foreground(colorMuted)

	content := c.form.View() + "\n" + hintStyle.Render("↑/↓: select • Enter: confirm • Esc: cancel")

	return selectorStyle.Render(content)
}
