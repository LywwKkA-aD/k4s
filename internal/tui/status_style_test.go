package tui

import (
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// TestStatusStyleOfEachLevel: an alert is coloured as an error, the worst
// news a poll brings.
func TestStatusStyleOfEachLevel(t *testing.T) {
	t.Parallel()

	look := func(s lipgloss.Style) string {
		return fmt.Sprint(s.GetForeground(), s.GetBold(), s.GetUnderline())
	}
	cases := []struct {
		name  string
		level status.Level
		want  lipgloss.Style
	}{
		{name: "info", level: status.Info, want: lipgloss.NewStyle()},
		{name: "ok", level: status.OK, want: styles.OK},
		{name: "warn", level: status.Warn, want: styles.Warn},
		{name: "error", level: status.Error, want: styles.Err},
		{name: "alert", level: status.Alert, want: styles.Err},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := look(statusStyle(tc.level)); got != look(tc.want) {
				t.Fatalf("statusStyle(%s) = %s, want %s", tc.name, got, look(tc.want))
			}
		})
	}
}
