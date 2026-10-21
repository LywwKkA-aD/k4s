package contexts_test

import (
	"os"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain points KUBECONFIG at a generated file with the contexts alpha
// (current) and beta, so the picker never reads the developer's own, and
// XDG_STATE_HOME at an empty directory so the root model the flow tests
// drive finds no saved port-forwards.
func TestMain(m *testing.M) {
	os.Exit(tuitest.Isolate(m, alpha, beta))
}
