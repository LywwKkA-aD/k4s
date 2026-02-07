package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// sshHostItem implements list.Item for SSH hosts
type sshHostItem struct {
	host domain.SSHHost
}

func (i sshHostItem) FilterValue() string { return i.host.Name }

// sshHostDelegate renders SSH host list items
type sshHostDelegate struct {
	styles Styles
}

func (d sshHostDelegate) Height() int                             { return 2 }
func (d sshHostDelegate) Spacing() int                            { return 1 }
func (d sshHostDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d sshHostDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(sshHostItem)
	if !ok {
		return
	}

	host := item.host
	port := host.Port
	if port == 0 {
		port = 22
	}

	connectionStr := fmt.Sprintf("%s@%s:%d", host.User, host.Host, port)

	prefix := lipgloss.NewStyle().Foreground(colorPrimary).Render("▌")
	bgStyle := lipgloss.NewStyle().Background(colorBgHighlight)

	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
		connStyle := lipgloss.NewStyle().Foreground(colorMuted)
		line1 := bgStyle.Render(fmt.Sprintf("%s %s", prefix, nameStyle.Render(host.Name)))
		line2 := bgStyle.Render(fmt.Sprintf("  %s", connStyle.Render(connectionStr)))
		fmt.Fprintf(w, "%s\n%s", line1, line2)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		connStyle := lipgloss.NewStyle().Foreground(colorMuted)
		fmt.Fprintf(w, "  %s\n  %s", nameStyle.Render(host.Name), connStyle.Render(connectionStr))
	}
}

// newSSHHostList creates a list model for SSH hosts
func newSSHHostList(hosts []domain.SSHHost, width, height int, styles Styles) list.Model {
	items := make([]list.Item, len(hosts))
	for i, host := range hosts {
		items[i] = sshHostItem{host: host}
	}

	delegate := sshHostDelegate{styles: styles}
	l := list.New(items, delegate, width, height)
	l.Title = "SSH Hosts"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.Title = styles.Title
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(colorPrimary)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(colorPrimary)

	return l
}

// updateSSHHostList updates the SSH host list items
func updateSSHHostList(l *list.Model, hosts []domain.SSHHost) {
	items := make([]list.Item, len(hosts))
	for i, host := range hosts {
		items[i] = sshHostItem{host: host}
	}
	l.SetItems(items)
}
