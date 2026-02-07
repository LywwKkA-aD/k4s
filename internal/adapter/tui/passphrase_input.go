package tui

import (
	"strings"

	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PassphraseInput is a modal dialog for entering SSH passphrase
type PassphraseInput struct {
	visible    bool
	hostName   string
	width      int
	passphrase string
	form       *huh.Form
}

// NewPassphraseInput creates a new passphrase input dialog
func NewPassphraseInput() PassphraseInput {
	return PassphraseInput{}
}

// Show displays the passphrase input for the given host and returns a tea.Cmd.
func (p *PassphraseInput) Show(hostName string) tea.Cmd {
	p.visible = true
	p.hostName = hostName
	p.passphrase = ""

	p.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("SSH Passphrase Required").
				Description("Host: "+hostName).
				Placeholder("Enter passphrase...").
				EchoMode(huh.EchoModePassword).
				Value(&p.passphrase),
		),
	).WithTheme(K4sHuhTheme()).WithShowHelp(false)
	return p.form.Init()
}

// Hide hides the passphrase input
func (p *PassphraseInput) Hide() {
	p.visible = false
	p.form = nil
	p.passphrase = ""
}

// IsVisible returns true if the dialog is visible
func (p *PassphraseInput) IsVisible() bool {
	return p.visible
}

// SetWidth sets the dialog width
func (p *PassphraseInput) SetWidth(width int) {
	p.width = width
}

// Update handles input messages, returns (passphrase, submitted, cancelled, cmd)
func (p *PassphraseInput) Update(msg tea.Msg) (string, bool, bool, tea.Cmd) {
	if p.form == nil {
		return "", false, false, nil
	}

	// Handle esc for cancel
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			return "", false, true, nil
		}
	}

	model, cmd := p.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		p.form = f
	}

	if p.form.State == huh.StateCompleted {
		return p.passphrase, true, false, cmd
	}
	if p.form.State == huh.StateAborted {
		return "", false, true, cmd
	}

	return "", false, false, cmd
}

// View renders the passphrase input dialog
func (p *PassphraseInput) View() string {
	if !p.visible || p.form == nil {
		return ""
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(50)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorMuted)

	var sb strings.Builder
	sb.WriteString(p.form.View())
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("Enter: submit • Esc: cancel"))

	return dialogStyle.Render(sb.String())
}
