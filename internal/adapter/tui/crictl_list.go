package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/adapter/ssh"
)

// crictlContainerItem implements list.Item for crictl containers
type crictlContainerItem struct {
	container ssh.CrictlContainer
}

func (i crictlContainerItem) FilterValue() string {
	// Allow filtering by container name, pod name, or namespace
	return i.container.Name + " " + i.container.PodName + " " + i.container.Namespace
}

// crictlContainerDelegate renders crictl container list items
type crictlContainerDelegate struct {
	styles Styles
}

func (d crictlContainerDelegate) Height() int                             { return 1 }
func (d crictlContainerDelegate) Spacing() int                            { return 0 }
func (d crictlContainerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d crictlContainerDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(crictlContainerItem)
	if !ok {
		return
	}

	c := item.container

	// Status color
	var statusStyle lipgloss.Style
	switch c.State {
	case "running":
		statusStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	case "exited":
		statusStyle = lipgloss.NewStyle().Foreground(colorMuted)
	case "created":
		statusStyle = lipgloss.NewStyle().Foreground(colorWarning)
	default:
		statusStyle = lipgloss.NewStyle().Foreground(colorMuted)
	}

	// Pad plain text FIRST, then apply styling
	// Columns: NAME(25) POD(30) NS(15) STATE(10) AGE(6)
	namePadded := fmt.Sprintf("%-25s", truncateString(c.Name, 25))
	podPadded := fmt.Sprintf("%-30s", truncateString(c.PodName, 30))
	nsPadded := fmt.Sprintf("%-15s", truncateString(c.Namespace, 15))
	statePadded := fmt.Sprintf("%-10s", c.State)
	agePadded := fmt.Sprintf("%-6s", c.Created)

	// Apply colors after padding
	stateStyled := statusStyle.Render(statePadded)
	ageStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(agePadded)
	nsStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(nsPadded)

	prefix := lipgloss.NewStyle().Foreground(colorPrimary).Render("▌")

	var line string
	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
		podStyle := lipgloss.NewStyle().Foreground(colorSecondary)
		line = fmt.Sprintf("%s %s %s %s %s %s", prefix, nameStyle.Render(namePadded), podStyle.Render(podPadded), nsStyled, stateStyled, ageStyled)
		line = lipgloss.NewStyle().Background(colorBgHighlight).Render(line)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		podStyle := lipgloss.NewStyle().Foreground(colorMuted)
		line = fmt.Sprintf("  %s %s %s %s %s", nameStyle.Render(namePadded), podStyle.Render(podPadded), nsStyled, stateStyled, ageStyled)
	}

	fmt.Fprint(w, line)
}

// newCrictlContainerList creates a list model for crictl containers
func newCrictlContainerList(containers []ssh.CrictlContainer, width, height int, styles Styles) list.Model {
	items := make([]list.Item, len(containers))
	for i, c := range containers {
		items[i] = crictlContainerItem{container: c}
	}

	delegate := crictlContainerDelegate{styles: styles}
	l := list.New(items, delegate, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(colorPrimary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(colorPrimary)

	return l
}

// updateCrictlContainerList updates the crictl container list
func updateCrictlContainerList(l *list.Model, containers []ssh.CrictlContainer) {
	items := make([]list.Item, len(containers))
	for i, c := range containers {
		items[i] = crictlContainerItem{container: c}
	}
	l.SetItems(items)
}
