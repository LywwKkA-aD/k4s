package forwards

import (
	"fmt"
	"strings"
)

// decorateError appends the captured stderr from the portforwarder to
// the raw error when it carries useful detail. The unwrapped error
// from ForwardPorts is often just "lost connection to pod" — the real
// reason ("bind: permission denied") lives in the captured output.
func decorateError(err error, output func() string) error {
	if output == nil {
		return err
	}
	extra := strings.TrimSpace(output())
	if extra == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, condense(extra))
}

// condense trims long stderr blobs to one line that is actually useful
// in a status column. Looks for well-known bind errors first; falls
// back to the first non-empty line.
func condense(s string) string {
	for _, sub := range []string{"bind: permission denied", "bind: address already in use"} {
		if strings.Contains(s, sub) {
			return sub
		}
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}

// friendlyBindError translates a raw net.Listen error into a one-liner
// the user can act on. Falls through with a generic message when the
// OS does not surface a recognised string.
func friendlyBindError(err error, port uint16) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "permission denied"):
		return fmt.Errorf("permission denied binding local port %d (ports < 1024 need root)", port)
	case strings.Contains(s, "address already in use"):
		return fmt.Errorf("local port %d already in use", port)
	}
	return fmt.Errorf("cannot bind local port %d: %w", port, err)
}
