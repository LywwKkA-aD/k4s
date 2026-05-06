package k8s

import (
	"fmt"
	"time"
)

// HumanizeDuration mirrors kubectl's "5s", "5m", "2h", "3d" output style.
// Used by the pods table and the describe view so they agree.
func HumanizeDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
