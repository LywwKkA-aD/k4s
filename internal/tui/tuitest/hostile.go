package tuitest

import (
	"strings"
	"testing"
)

// Hostile is name with what a crafted object or kubeconfig name can carry
// woven in: an OSC 52 clipboard write, a screen clear, bidi overrides and
// isolates, zero-width characters and a tag character. Cleaned
// (sanitize.Line or sanitize.Cell) it reads name again, so a test can look
// for name on screen.
func Hostile(name string) string {
	half := len(name) / 2
	return "\u202e" + name[:half] + "\x1b]52;c;ZXZpbA==\a\u200b" + name[half:] +
		"\x1b[2J\u2066\u200d\U000E0041\u2069"
}

// hostileMarks are the parts of Hostile no frame may carry. A frame's own
// styling is SGR (ESC [ ... m), which none of them is.
var hostileMarks = []string{
	"\x1b]", "\a", "\x1b[2J", "\x1b\\",
	"\u202e", "\u2066", "\u2069", "\u200b", "\u200d", "\U000E0041",
}

// AssertInert fails t when frame, a rendered view with its escape codes
// (Frame), carries any part of what Hostile weaves into a name.
func AssertInert(t testing.TB, frame string) {
	t.Helper()
	for _, bad := range hostileMarks {
		if i := strings.Index(frame, bad); i >= 0 {
			t.Fatalf("the frame carries %q: %q", bad, frame[max(i-60, 0):min(i+40, len(frame))])
		}
	}
}
