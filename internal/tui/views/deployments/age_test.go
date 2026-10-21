package deployments_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestAgeReadsLikeKubectl: the demo deployments are 90 minutes old, which
// kubectl prints as 90m, not 1h.
func TestAgeReadsLikeKubectl(t *testing.T) {
	t.Parallel()

	screen := tuitest.Screen(mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo))
	if !strings.Contains(screen, " 90m") {
		t.Errorf("no kubectl-style age in:\n%s", screen)
	}
}
