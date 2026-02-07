package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// deploymentItem implements list.Item for deployments
type deploymentItem struct {
	deployment domain.Deployment
}

func (i deploymentItem) FilterValue() string { return i.deployment.Name }

// deploymentDelegate renders deployment list items
type deploymentDelegate struct {
	styles Styles
}

func (d deploymentDelegate) Height() int                             { return 1 }
func (d deploymentDelegate) Spacing() int                            { return 0 }
func (d deploymentDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d deploymentDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(deploymentItem)
	if !ok {
		return
	}

	dep := item.deployment

	// Status color based on ready state
	var statusStyle lipgloss.Style
	if dep.ReadyReplicas == dep.Replicas && dep.Replicas > 0 {
		statusStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	} else if dep.ReadyReplicas == 0 {
		statusStyle = lipgloss.NewStyle().Foreground(colorError)
	} else {
		statusStyle = lipgloss.NewStyle().Foreground(colorWarning)
	}

	// Pad plain text FIRST, then apply styling
	namePadded := fmt.Sprintf("%-40s", truncateString(dep.Name, 40))
	readyPadded := fmt.Sprintf("%-10s", dep.Ready)
	upToDatePadded := fmt.Sprintf("%-10d", dep.UpToDate)
	availablePadded := fmt.Sprintf("%-10d", dep.Available)
	agePadded := dep.Age

	// Apply colors after padding
	readyStyled := statusStyle.Render(readyPadded)
	upToDateStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(upToDatePadded)
	availableStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(availablePadded)
	ageStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(agePadded)

	// Cursor and selection styling
	prefix := lipgloss.NewStyle().Foreground(colorPrimary).Render("▌")

	var line string
	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
		line = fmt.Sprintf("%s %s %s %s %s %s",
			prefix, nameStyle.Render(namePadded), readyStyled, upToDateStyled, availableStyled, ageStyled)
		line = lipgloss.NewStyle().Background(colorBgHighlight).Render(line)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		line = fmt.Sprintf("  %s %s %s %s %s",
			nameStyle.Render(namePadded), readyPadded, upToDateStyled, availableStyled, ageStyled)
	}

	fmt.Fprint(w, line)
}

// newDeploymentList creates a list model for deployments
func newDeploymentList(deployments []domain.Deployment, width, height int, styles Styles) list.Model {
	items := make([]list.Item, len(deployments))
	for i, dep := range deployments {
		items[i] = deploymentItem{deployment: dep}
	}

	delegate := deploymentDelegate{styles: styles}
	l := list.New(items, delegate, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(colorPrimary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(colorPrimary)

	return l
}

// updateDeploymentList updates the deployment list items while preserving selection
func updateDeploymentList(l *list.Model, deployments []domain.Deployment) {
	currentIndex := l.Index()
	var currentName string
	if item, ok := l.SelectedItem().(deploymentItem); ok {
		currentName = item.deployment.Name
	}

	items := make([]list.Item, len(deployments))
	newIndex := 0
	for i, dep := range deployments {
		items[i] = deploymentItem{deployment: dep}
		if dep.Name == currentName {
			newIndex = i
		}
	}

	l.SetItems(items)

	if currentName != "" {
		l.Select(newIndex)
	} else if currentIndex < len(items) {
		l.Select(currentIndex)
	}
}
