package nodes_test

import (
	"os"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain keeps the root model these tests drive away from the
// developer's machine: no saved port-forward opens the restore popup and
// no real kubeconfig is read.
func TestMain(m *testing.M) {
	os.Exit(tuitest.Isolate(m))
}
