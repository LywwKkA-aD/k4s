package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScaleDialog is a dialog for scaling deployments
type ScaleDialog struct {
	deployName string
	current    int32
	target     int32
	inputValue string
	visible    bool
	width      int
	form       *huh.Form
}

// NewScaleDialog creates a new scale dialog
func NewScaleDialog() ScaleDialog {
	return ScaleDialog{}
}

// Show displays the scale dialog and returns a tea.Cmd to initialise the form.
func (d *ScaleDialog) Show(deployName string, currentReplicas int32) tea.Cmd {
	d.deployName = deployName
	d.current = currentReplicas
	d.target = currentReplicas
	d.visible = true
	d.inputValue = fmt.Sprintf("%d", currentReplicas)

	d.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Scale Deployment").
				Description(fmt.Sprintf("%s (current: %d)", truncateString(deployName, 40), currentReplicas)).
				Placeholder("0").
				Value(&d.inputValue).
				Validate(func(s string) error {
					val, err := strconv.ParseInt(s, 10, 32)
					if err != nil {
						return fmt.Errorf("invalid number")
					}
					if val < 0 {
						return fmt.Errorf("must be >= 0")
					}
					if val > 1000 {
						return fmt.Errorf("max is 1000")
					}
					return nil
				}),
		),
	).WithTheme(K4sHuhTheme()).WithShowHelp(false)
	return d.form.Init()
}

// Hide hides the scale dialog
func (d *ScaleDialog) Hide() {
	d.visible = false
	d.deployName = ""
	d.form = nil
}

// IsVisible returns whether the dialog is visible
func (d *ScaleDialog) IsVisible() bool {
	return d.visible
}

// DeploymentName returns the deployment name
func (d *ScaleDialog) DeploymentName() string {
	return d.deployName
}

// TargetReplicas returns the target replica count
func (d *ScaleDialog) TargetReplicas() int32 {
	return d.target
}

// CurrentReplicas returns the current replica count
func (d *ScaleDialog) CurrentReplicas() int32 {
	return d.current
}

// SetWidth sets the dialog width
func (d *ScaleDialog) SetWidth(width int) {
	d.width = width
}

// Update handles key messages for the dialog
func (d *ScaleDialog) Update(msg tea.Msg) (confirmed bool, cancelled bool, cmd tea.Cmd) {
	if !d.visible || d.form == nil {
		return false, false, nil
	}

	// Handle esc for cancel
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			return false, true, nil
		}
	}

	model, formCmd := d.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		d.form = f
	}

	if d.form.State == huh.StateCompleted {
		// Parse validated value
		val, err := strconv.ParseInt(d.inputValue, 10, 32)
		if err == nil {
			d.target = int32(val)
		}
		return true, false, formCmd
	}
	if d.form.State == huh.StateAborted {
		return false, true, formCmd
	}

	return false, false, formCmd
}

// View renders the scale dialog
func (d *ScaleDialog) View() string {
	if !d.visible || d.form == nil {
		return ""
	}

	dialogWidth := 45
	if d.width > 0 && d.width < 55 {
		dialogWidth = d.width - 10
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(dialogWidth).
		Align(lipgloss.Center)

	hintStyle := lipgloss.NewStyle().
		Foreground(colorMuted)

	content := d.form.View() + "\n" + hintStyle.Render("Enter: confirm • Esc: cancel")

	return dialogStyle.Render(content)
}
