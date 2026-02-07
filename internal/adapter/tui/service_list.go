package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// serviceItem implements list.Item for services
type serviceItem struct {
	service domain.Service
}

func (i serviceItem) FilterValue() string { return i.service.Name }

// serviceDelegate renders service list items
type serviceDelegate struct {
	styles Styles
}

func (d serviceDelegate) Height() int                             { return 1 }
func (d serviceDelegate) Spacing() int                            { return 0 }
func (d serviceDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d serviceDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(serviceItem)
	if !ok {
		return
	}

	svc := item.service

	// Type color based on service type
	var typeStyle lipgloss.Style
	switch svc.Type {
	case domain.ServiceTypeLoadBalancer:
		if svc.ExternalIP != "<pending>" && svc.ExternalIP != "<none>" {
			typeStyle = lipgloss.NewStyle().Foreground(colorSuccess)
		} else {
			typeStyle = lipgloss.NewStyle().Foreground(colorWarning)
		}
	case domain.ServiceTypeNodePort:
		typeStyle = lipgloss.NewStyle().Foreground(colorSecondary)
	default:
		typeStyle = lipgloss.NewStyle().Foreground(colorMuted)
	}

	// Pad plain text FIRST, then apply styling
	namePadded := fmt.Sprintf("%-30s", truncateString(svc.Name, 30))
	typePadded := fmt.Sprintf("%-14s", svc.Type)
	clusterIPPadded := fmt.Sprintf("%-16s", svc.ClusterIP)
	externalIPPadded := fmt.Sprintf("%-20s", truncateString(svc.ExternalIP, 20))
	portsPadded := fmt.Sprintf("%-20s", truncateString(svc.Ports, 20))
	agePadded := svc.Age

	// Apply colors after padding
	typeStyled := typeStyle.Render(typePadded)
	clusterIPStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(clusterIPPadded)
	externalIPStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(externalIPPadded)
	portsStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(portsPadded)
	ageStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(agePadded)

	// Cursor and selection styling
	prefix := lipgloss.NewStyle().Foreground(colorPrimary).Render("▌")

	var line string
	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
		line = fmt.Sprintf("%s %s %s %s %s %s %s",
			prefix, nameStyle.Render(namePadded), typeStyled, clusterIPStyled, externalIPStyled, portsStyled, ageStyled)
		line = lipgloss.NewStyle().Background(colorBgHighlight).Render(line)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		line = fmt.Sprintf("  %s %s %s %s %s %s",
			nameStyle.Render(namePadded), typePadded, clusterIPStyled, externalIPStyled, portsStyled, ageStyled)
	}

	fmt.Fprint(w, line)
}

// newServiceList creates a list model for services
func newServiceList(services []domain.Service, width, height int, styles Styles) list.Model {
	items := make([]list.Item, len(services))
	for i, svc := range services {
		items[i] = serviceItem{service: svc}
	}

	delegate := serviceDelegate{styles: styles}
	l := list.New(items, delegate, width, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(colorPrimary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(colorPrimary)

	return l
}

// updateServiceList updates the service list items while preserving selection
func updateServiceList(l *list.Model, services []domain.Service) {
	currentIndex := l.Index()
	var currentName string
	if item, ok := l.SelectedItem().(serviceItem); ok {
		currentName = item.service.Name
	}

	items := make([]list.Item, len(services))
	newIndex := 0
	for i, svc := range services {
		items[i] = serviceItem{service: svc}
		if svc.Name == currentName {
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
