package forwards

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
)

// recoverSupervisor turns a panic on a supervisor goroutine into an error
// on its forward and writes the stack to the k4s log. Bubble Tea recovers
// only the goroutines it runs; a panic here would end k4s with the
// terminal still in raw mode.
func (m *Manager) recoverSupervisor(id string, attempt uint64, sess *k8s.PortForwardSession) {
	r := recover()
	if r == nil {
		return
	}
	m.logf("port-forward supervisor of forward %s panicked: %v\n%s", id, r, debug.Stack())
	sess.Close()
	m.fail(id, attempt, sess, internalError(), k8s.TunnelError{})
}

// internalError is what a forward shows after a bug in k4s ended it.
func internalError() error {
	return fmt.Errorf("%w (see %s)", k8s.ErrInternal, logPath())
}

// logPath is where k4s writes its log, as the user would type it.
func logPath() string {
	home, _ := os.UserHomeDir()
	path := logsink.ResolvePath(os.Getenv("XDG_STATE_HOME"), home)
	if path == "" {
		return "the k4s log"
	}
	return displayPath(path)
}
