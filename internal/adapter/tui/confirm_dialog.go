package tui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmAction represents the type of action being confirmed
type ConfirmAction int

const (
	ConfirmActionNone ConfirmAction = iota
	ConfirmActionDeletePod
	ConfirmActionRestartPod
	ConfirmActionDeleteDeployment
	ConfirmActionRestartDeployment
)

// ConfirmDialog is a confirmation dialog model
type ConfirmDialog struct {
	action     ConfirmAction
	title      string
	message    string
	targetName string
	visible    bool
	width      int
	confirmed  bool
	form       *huh.Form
}

// NewConfirmDialog creates a new confirmation dialog
func NewConfirmDialog() ConfirmDialog {
	return ConfirmDialog{}
}

// Show displays the confirmation dialog and returns a tea.Cmd to initialise the form.
func (d *ConfirmDialog) Show(action ConfirmAction, targetName string) tea.Cmd {
	d.action = action
	d.targetName = targetName
	d.visible = true
	d.confirmed = false

	switch action {
	case ConfirmActionDeletePod:
		d.title = "Delete Pod"
		d.message = fmt.Sprintf("Are you sure you want to delete pod '%s'?", targetName)
	case ConfirmActionRestartPod:
		d.title = "Restart Pod"
		d.message = fmt.Sprintf("Are you sure you want to restart pod '%s'?\n(This will delete the pod; the controller will recreate it)", targetName)
	case ConfirmActionDeleteDeployment:
		d.title = "Delete Deployment"
		d.message = fmt.Sprintf("Are you sure you want to delete deployment '%s'?\n(All associated pods will be terminated)", targetName)
	case ConfirmActionRestartDeployment:
		d.title = "Restart Deployment"
		d.message = fmt.Sprintf("Are you sure you want to restart deployment '%s'?\n(This triggers a rolling restart of all pods)", targetName)
	default:
		d.title = "Confirm"
		d.message = "Are you sure?"
	}

	d.form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(d.title).
				Description(d.message).
				Affirmative("Yes").
				Negative("No").
				Value(&d.confirmed),
		),
	).WithTheme(K4sHuhTheme()).WithShowHelp(false)
	return d.form.Init()
}

// Hide hides the confirmation dialog
func (d *ConfirmDialog) Hide() {
	d.visible = false
	d.action = ConfirmActionNone
	d.targetName = ""
	d.form = nil
}

// IsVisible returns whether the dialog is visible
func (d *ConfirmDialog) IsVisible() bool {
	return d.visible
}

// Action returns the current action being confirmed
func (d *ConfirmDialog) Action() ConfirmAction {
	return d.action
}

// TargetName returns the target name for the action
func (d *ConfirmDialog) TargetName() string {
	return d.targetName
}

// SetWidth sets the dialog width
func (d *ConfirmDialog) SetWidth(width int) {
	d.width = width
}

// Update handles key messages for the dialog
func (d *ConfirmDialog) Update(msg tea.Msg) (confirmed bool, cancelled bool, cmd tea.Cmd) {
	if !d.visible || d.form == nil {
		return false, false, nil
	}

	// Quick shortcuts: Y confirms, N/Esc cancels — bypass the form entirely
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "y", "Y":
			return true, false, nil
		case "n", "N", "esc":
			return false, true, nil
		}
	}

	// Delegate to the huh form for Enter / arrow-key interaction
	model, formCmd := d.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		d.form = f
	}

	if d.form.State == huh.StateCompleted {
		return d.confirmed, !d.confirmed, formCmd
	}
	if d.form.State == huh.StateAborted {
		return false, true, formCmd
	}

	return false, false, formCmd
}

// View renders the confirmation dialog
func (d *ConfirmDialog) View() string {
	if !d.visible || d.form == nil {
		return ""
	}

	dialogWidth := 50
	if d.width > 0 && d.width < 60 {
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

	content := d.form.View() + "\n" + hintStyle.Render("Y: confirm • N/Esc: cancel")

	return dialogStyle.Render(content)
}
