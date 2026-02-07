package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// namespaceItem implements list.Item for namespaces
type namespaceItem struct {
	namespace domain.Namespace
}

func (i namespaceItem) FilterValue() string { return i.namespace.Name }

// namespaceDelegate renders namespace list items
type namespaceDelegate struct {
	styles Styles
}

func (d namespaceDelegate) Height() int                             { return 1 }
func (d namespaceDelegate) Spacing() int                            { return 0 }
func (d namespaceDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d namespaceDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(namespaceItem)
	if !ok {
		return
	}

	ns := item.namespace

	// Status color
	var statusStyle lipgloss.Style
	switch ns.Status {
	case "Active":
		statusStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	case "Terminating":
		statusStyle = lipgloss.NewStyle().Foreground(colorWarning)
	default:
		statusStyle = lipgloss.NewStyle().Foreground(colorMuted)
	}

	// Pad plain text FIRST, then apply styling (ANSI codes break fmt width calculation)
	namePadded := fmt.Sprintf("%-45s", truncateString(ns.Name, 45))
	statusPadded := fmt.Sprintf("%-12s", ns.Status)
	agePadded := ns.Age

	// Apply colors after padding
	statusStyled := statusStyle.Render(statusPadded)
	ageStyled := lipgloss.NewStyle().Foreground(colorMuted).Render(agePadded)

	// Cursor and selection styling
	prefix := lipgloss.NewStyle().Foreground(colorPrimary).Render("▌")

	var line string
	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
		line = fmt.Sprintf("%s %s %s %s", prefix, nameStyle.Render(namePadded), statusStyled, ageStyled)
		line = lipgloss.NewStyle().Background(colorBgHighlight).Render(line)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		line = fmt.Sprintf("  %s %s %s", nameStyle.Render(namePadded), statusStyled, ageStyled)
	}

	fmt.Fprint(w, line)
}

// newNamespaceList creates a list model for namespaces
func newNamespaceList(namespaces []domain.Namespace, width, height int, styles Styles) list.Model {
	items := make([]list.Item, len(namespaces))
	for i, ns := range namespaces {
		items[i] = namespaceItem{namespace: ns}
	}

	delegate := namespaceDelegate{styles: styles}
	l := list.New(items, delegate, width, height)
	l.SetShowTitle(false)      // We render our own title
	l.SetShowStatusBar(false)  // We render our own status
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(colorPrimary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(colorPrimary)

	return l
}

// updateNamespaceList updates the namespace list items
func updateNamespaceList(l *list.Model, namespaces []domain.Namespace) {
	items := make([]list.Item, len(namespaces))
	for i, ns := range namespaces {
		items[i] = namespaceItem{namespace: ns}
	}
	l.SetItems(items)
}
