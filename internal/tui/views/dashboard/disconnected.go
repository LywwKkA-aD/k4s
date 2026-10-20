package dashboard

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// connectHint is the generic next step shown whenever there is no client.
const connectHint = "set KUBECONFIG or create ~/.kube/config"

// disconnectedView explains why there is no cluster connection. loadErr comes
// from the kubeconfig loader, whose messages carry file paths and context
// names but never credential values.
func disconnectedView(loadErr error, home string) string {
	reason := "no kubeconfig loaded"
	if loadErr != nil {
		reason = "not connected: " + tildeHome(loadErr.Error(), home)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.Warn.Render(reason),
		styles.Hint.Render(connectHint),
	)
}

// tildeHome shortens every path under home to "~/...". Only whole path
// components match, so home /home/al leaves /home/alice untouched.
func tildeHome(s, home string) string {
	sep := string(filepath.Separator)
	home = strings.TrimSuffix(home, sep)
	if home == "" {
		return s
	}
	return strings.ReplaceAll(s, home+sep, "~"+sep)
}
