// Package logsink keeps library diagnostics off the terminal while the TUI
// owns it. klog, apimachinery's HandleError, client-go API warnings and the
// standard log package are routed to a private log file; exec credential
// plugin stderr is held in memory until the TUI exits.
package logsink

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// Sink is the log file every library diagnostic goes to. It counts what it
// receives so the user can be told about it after the TUI exits. A Sink is
// safe for concurrent use.
type Sink struct {
	path    string // "" when discarding
	home    string
	openErr error

	mu       sync.Mutex
	w        io.Writer
	file     *os.File
	messages int
	closed   bool
}

// Open opens the log at ResolvePath($XDG_STATE_HOME, home). It never fails:
// if the file cannot be opened, the Sink discards what it receives and
// Summary says so.
func Open() *Sink {
	home, _ := os.UserHomeDir()
	return OpenAt(ResolvePath(os.Getenv("XDG_STATE_HOME"), home), home)
}

// OpenAt opens the log at path; home only shortens the path in Summary. An
// empty or unopenable path yields a Sink that discards.
func OpenAt(path, home string) *Sink {
	if path == "" {
		return &Sink{home: home, w: io.Discard, openErr: errors.New("no state or home directory")}
	}
	f, err := openLogFile(path)
	if err != nil {
		return &Sink{home: home, w: io.Discard, openErr: err}
	}
	return &Sink{path: path, home: home, w: f, file: f}
}

// Write appends p to the log. Each non-empty write counts as one message:
// klog, the warning writer and the error handler each emit an entry in a
// single call, even when it spans several lines.
func (s *Sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(p) == 0 {
		return len(p), nil
	}
	s.messages++
	return s.w.Write(p)
}

// Messages reports how many messages the Sink has received.
func (s *Sink) Messages() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages
}

// Path is the log file path, or "" when the Sink discards.
func (s *Sink) Path() string { return s.path }

// Summary is the line to print once the TUI has exited, or "" when nothing
// was logged.
func (s *Sink) Summary() string {
	n := s.Messages()
	if n == 0 {
		return ""
	}
	noun := "messages"
	if n == 1 {
		noun = "message"
	}
	if s.path == "" {
		return fmt.Sprintf("k4s: %d %s dropped (log file unavailable: %v)", n, noun, s.openErr)
	}
	return fmt.Sprintf("k4s: %d %s logged to %s", n, noun, displayPath(s.path, s.home))
}

// Close closes the log file. Later writes are discarded; Close is
// idempotent.
func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.w = io.Discard
	if s.file == nil {
		return nil
	}
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("close log file: %w", err)
	}
	return nil
}
