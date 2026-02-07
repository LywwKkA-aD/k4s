package tui

import (
	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

const allPodsValue = "__all_pods__"

// PodMultiSelector is a multi-select dialog for choosing multiple pods
type PodMultiSelector struct {
	visible  bool
	width    int
	selected []string
	allPods  []string // all pod names for expanding "All Pods"
	form     *huh.Form
}

// NewPodMultiSelector creates a new pod multi-selector
func NewPodMultiSelector() PodMultiSelector {
	return PodMultiSelector{}
}

// Show displays the pod multi-selector and returns a tea.Cmd
func (s *PodMultiSelector) Show(pods []domain.Pod) tea.Cmd {
	s.visible = true
	s.selected = nil

	s.allPods = make([]string, len(pods))
	opts := make([]huh.Option[string], 0, len(pods)+1)

	// "All Pods" option first
	opts = append(opts, huh.NewOption("* All Pods", allPodsValue))

	for i, pod := range pods {
		s.allPods[i] = pod.Name
		opts = append(opts, huh.NewOption(pod.Name, pod.Name))
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select Pods for Multi-Log").
				Options(opts...).
				Value(&s.selected).
				Filterable(true),
		),
	).WithTheme(K4sHuhTheme()).WithShowHelp(false)
	return s.form.Init()
}

// Hide hides the pod multi-selector
func (s *PodMultiSelector) Hide() {
	s.visible = false
	s.form = nil
}

// IsVisible returns whether the selector is visible
func (s *PodMultiSelector) IsVisible() bool {
	return s.visible
}

// SetWidth sets the selector width
func (s *PodMultiSelector) SetWidth(width int) {
	s.width = width
}

// SelectedPods returns the selected pod names, expanding "All Pods" if selected
func (s *PodMultiSelector) SelectedPods() []string {
	for _, v := range s.selected {
		if v == allPodsValue {
			return s.allPods
		}
	}
	return s.selected
}

// Update handles messages for the selector
func (s *PodMultiSelector) Update(msg tea.Msg) (confirmed bool, cancelled bool, cmd tea.Cmd) {
	if !s.visible || s.form == nil {
		return false, false, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			return false, true, nil
		}
	}

	model, formCmd := s.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		s.form = f
	}

	if s.form.State == huh.StateCompleted {
		return true, false, formCmd
	}
	if s.form.State == huh.StateAborted {
		return false, true, formCmd
	}

	return false, false, formCmd
}

// View renders the pod multi-selector
func (s *PodMultiSelector) View() string {
	if !s.visible || s.form == nil {
		return ""
	}

	dialogWidth := 55
	if s.width > 0 && s.width < 65 {
		dialogWidth = s.width - 10
	}

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(dialogWidth)

	hintStyle := lipgloss.NewStyle().Foreground(colorMuted)
	content := s.form.View() + "\n" + hintStyle.Render("Space: toggle  Enter: confirm  Esc: cancel")

	return dialogStyle.Render(content)
}
