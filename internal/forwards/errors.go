package forwards

import (
	"fmt"
	"math"
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

// knownCauses are the endings of tunnel errors worth a DETAIL cell on
// their own, from the kubelet's dial inside the pod or the connection to
// it.
var knownCauses = []string{
	"connection refused",
	"connection reset by peer",
	"no route to host",
	"network is unreachable",
	"i/o timeout",
	"broken pipe",
}

// tunnelCause sums up a failure on one of a forward's connections. The
// kubelet's message runs to a few hundred bytes ("error forwarding port 80
// to pod …: failed to connect to localhost:80 inside namespace …: connect:
// connection refused"); its cause is the part a user acts on.
func tunnelCause(msg string) string {
	for _, c := range knownCauses {
		if strings.Contains(msg, c) {
			return c
		}
	}
	msg = strings.TrimSpace(msg)
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

// unprivilegedOffset moves a privileged port into the user range the way
// people tend to by hand: 80 becomes 8080 and 443 becomes 8443.
const unprivilegedOffset = 8000

// FriendlyBindError translates a failed listen on local port into a
// one-liner the user can act on: a privileged port gets a suggested
// alternative, a busy one a hint to pick another. Unrecognised errors keep
// their own text. A nil err yields "".
func FriendlyBindError(err error, port uint16) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "permission denied"):
		return fmt.Sprintf("permission denied binding local port %d (%s)", port, privilegedHint(port))
	case strings.Contains(s, "address already in use"):
		return fmt.Sprintf("local port %d already in use — pick another", port)
	}
	return fmt.Sprintf("cannot bind local port %d: %v", port, err)
}

func privilegedHint(port uint16) string {
	alt := int(port) + unprivilegedOffset
	if alt > math.MaxUint16 {
		return "ports < 1024 need root"
	}
	return fmt.Sprintf("ports < 1024 need root — try %d", alt)
}
