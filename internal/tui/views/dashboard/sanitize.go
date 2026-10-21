package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The dashboard's own sanitizer until the shared M5.1 package lands: every
// string that comes from the cluster (names, statuses, event text, the
// user name, versions) goes through clean before it reaches layout, so a
// crafted object name or event message cannot drive the terminal.

// clean returns s as inert text: escape sequences (CSI, OSC such as the
// OSC 52 clipboard write, DCS and the rest) are removed whole, C0 and C1
// controls, DEL and bidi overrides are dropped, tabs and line breaks
// become spaces so every value stays on one line, and invalid UTF-8 shows
// as U+FFFD.
func clean(s string) string {
	if plainASCII(s) {
		return s
	}
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		case isControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleanArgs cleans every argument of a command into a new slice.
func cleanArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = clean(a)
	}
	return out
}

// plainASCII reports printable ASCII only, the common case, which needs no
// copy.
func plainASCII(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// isControl reports a rune a terminal may act on rather than print: C0,
// DEL, C1 and the bidi embedding, override and isolate controls.
func isControl(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}
