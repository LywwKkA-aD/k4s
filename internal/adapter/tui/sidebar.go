package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// sidebarMinWidth is the minimum width threshold to show the sidebar
const sidebarMinWidth = 90

// renderSidebar renders the right sidebar panel
func (a *App) renderSidebar() string {
	w := a.sidebarWidth
	innerW := w - 3 // account for left border + padding

	var sb strings.Builder

	// 1. Logo block
	sb.WriteString(renderLogo(innerW))
	sb.WriteString("\n")

	// 2. Subtitle
	subtitleStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sb.WriteString(subtitleStyle.Render("Kubernetes TUI for K3s"))
	sb.WriteString("\n\n")

	// 3. Cluster info
	sb.WriteString(a.renderSidebarClusterInfo())
	sb.WriteString("\n")

	// 4. Separator
	sb.WriteString(a.renderSidebarSeparator(innerW))
	sb.WriteString("\n")

	// 5. Navigation
	sb.WriteString(a.renderSidebarNavigation())

	// Wrap in sidebar style with left border
	sidebarStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.Border{Left: "│"}).
		BorderLeft(true).
		BorderForeground(colorBorder).
		PaddingLeft(1).
		PaddingRight(1).
		Width(w).
		Height(a.height - 4) // account for footer + app padding

	return sidebarStyle.Render(sb.String())
}

// renderSidebarClusterInfo renders the cluster connection info section
func (a *App) renderSidebarClusterInfo() string {
	var sb strings.Builder

	// Connection status indicator
	var indicator string
	var statusText string
	switch a.connectionStatus {
	case domain.StatusConnected:
		indicator = lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")
		statusText = "Connected"
	case domain.StatusConnecting:
		indicator = lipgloss.NewStyle().Foreground(colorWarning).Render("◉")
		statusText = "Connecting"
	case domain.StatusError:
		indicator = lipgloss.NewStyle().Foreground(colorError).Render("○")
		statusText = "Error"
	default:
		indicator = lipgloss.NewStyle().Foreground(colorMuted).Render("○")
		statusText = "Disconnected"
	}

	statusStyle := lipgloss.NewStyle().Foreground(colorText)
	sb.WriteString(fmt.Sprintf("%s %s", indicator, statusStyle.Render(statusText)))
	sb.WriteString("\n")

	// Cluster details
	if a.clusterInfo != nil {
		nameStyle := lipgloss.NewStyle().Foreground(colorText)
		mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)

		sb.WriteString(fmt.Sprintf("  %s", nameStyle.Render(a.clusterInfo.Context)))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  %s", mutedStyle.Render(a.clusterInfo.Namespace)))
		sb.WriteString("\n")
	}

	// Kubeconfig name
	if a.selectedConfig != nil {
		subtleStyle := lipgloss.NewStyle().Foreground(colorSubtle)
		sb.WriteString(fmt.Sprintf("  %s", subtleStyle.Render(a.selectedConfig.Name)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderSidebarSeparator renders a thin horizontal line
func (a *App) renderSidebarSeparator(width int) string {
	sepStyle := lipgloss.NewStyle().Foreground(colorDim)
	return sepStyle.Render(strings.Repeat("─", width))
}

// renderSidebarNavigation renders the navigation section
func (a *App) renderSidebarNavigation() string {
	var sb strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sb.WriteString(sectionStyle.Render("Navigation"))
	sb.WriteString("\n\n")

	type navItem struct {
		key   string
		label string
		views []ViewState // active when viewState matches any of these
	}

	items := []navItem{
		{"1", "Namespaces", []ViewState{ViewNamespaces}},
		{"2", "Pods", []ViewState{ViewPods, ViewPodDetails, ViewLogs}},
		{"3", "Deployments", []ViewState{ViewDeployments, ViewDeploymentDetails}},
		{"4", "Services", []ViewState{ViewServices, ViewServiceDetails}},
		{"5", "Events", []ViewState{ViewEvents}},
	}

	// Add SSH if configured
	if len(a.config.SSHHosts) > 0 {
		items = append(items, navItem{"9", "SSH", []ViewState{ViewSSHHosts, ViewSSHConnecting, ViewCrictlContainers, ViewCrictlLogs}})
	}

	activeStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(colorMuted)
	keyStyle := lipgloss.NewStyle().Foreground(colorSubtle)

	for _, item := range items {
		isActive := false
		for _, v := range item.views {
			if a.viewState == v {
				isActive = true
				break
			}
		}

		if isActive {
			sb.WriteString(fmt.Sprintf("▸ %s %s", activeStyle.Render(item.key), activeStyle.Render(item.label)))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %s", keyStyle.Render(item.key), inactiveStyle.Render(item.label)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
