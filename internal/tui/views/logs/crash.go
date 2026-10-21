package logs

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/logsink"
)

// crashLog is where the stack of a panic recovered in a stream goroutine
// goes. Bubble Tea only guards its own goroutines: a panic in one k4s
// starts would kill the process and leave the terminal in raw mode.
type crashLog struct {
	logger *log.Logger
}

// defaultCrashLog writes to the standard logger, which logsink.Install
// points at the k4s log.
func defaultCrashLog() crashLog {
	return crashLog{logger: log.Default()}
}

// internalError is a panic recovered in a stream goroutine: the view
// shows it as the stream's error and says it in the status line.
type internalError struct {
	log string
}

func (e internalError) Error() string {
	return "internal error (see " + e.log + ")"
}

// recovered logs the panic value v with the stack of the goroutine that
// panicked, and returns the error the view shows. Call it from the
// deferred function that recovered v.
func (c crashLog) recovered(where string, v any) error {
	c.logger.Printf("k4s: panic in %s: %v\n%s", where, v, debug.Stack())
	return internalError{log: logPath(c.logger.Writer())}
}

// guard is deferred by a stream goroutine: it recovers a panic and hands
// report the error to show in its place.
func (c crashLog) guard(where string, report func(error)) {
	if v := recover(); v != nil {
		report(c.recovered(where, v))
	}
}

// logPath names the file w writes to, home shortened to ~, when w is the
// k4s log sink.
func logPath(w io.Writer) string {
	s, ok := w.(*logsink.Sink)
	if !ok || s.Path() == "" {
		return "the k4s log"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return s.Path()
	}
	if rest, ok := strings.CutPrefix(s.Path(), filepath.Clean(home)+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return s.Path()
}
