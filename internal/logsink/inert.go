package logsink

import (
	"io"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// inertWriter passes each write through sanitize.Text, line breaks kept,
// before w sees it. klog lines, API warnings and HandleError text quote
// object names and whatever the API server said, and a later `cat` of the
// log would run an escape sequence among them. Clean text, nearly every
// write, goes through without a copy.
type inertWriter struct{ w io.Writer }

// Write reports all of p written once its inert form is: the caller's
// bytes are consumed even when the scrub dropped some of them.
func (i inertWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(i.w, sanitize.Text(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
