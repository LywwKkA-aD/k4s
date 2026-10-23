package dashboard

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// connectHint is the generic next step shown whenever there is no client.
const connectHint = "set KUBECONFIG or create ~/.kube/config"

// maxReasonLen caps the load error so a parser message quoting a long or
// multi-line fragment of the file cannot flood the dashboard.
const maxReasonLen = 300

// disconnectedView explains why there is no cluster connection. loadErr comes
// from the kubeconfig loader, whose messages carry file paths and context
// names but never credential values; they can quote the file, so the
// reason is made inert (clean) before it is drawn.
func disconnectedView(loadErr error, home string) string {
	reason := "no kubeconfig loaded"
	if loadErr != nil {
		reason = "not connected: " + oneLine(clean(tildeHome(loadErr.Error(), home)))
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

// oneLine collapses all whitespace runs, newlines included, to one space and
// caps the result at maxReasonLen runes.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxReasonLen {
		return string(r[:maxReasonLen]) + "…"
	}
	return s
}
