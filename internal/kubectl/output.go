package kubectl

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
)

// cappedBuffer keeps the first limit bytes written to it and drops the
// rest while still reporting success, so the copy from kubectl's pipe
// keeps draining. Each stream gets its own buffer and writer goroutine.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if room := b.limit - b.buf.Len(); n > room {
		b.truncated = true
		p = p[:max(room, 0)]
	}
	b.buf.Write(p)
	return n, nil
}

// escapes matches CSI and OSC terminal sequences and lone two-byte escapes,
// which an error message from the cluster can carry into the status line.
var escapes = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?|\x1b.?`)

// firstLine returns the first non-blank line of out with terminal escapes
// and other control characters removed.
func firstLine(out []byte) string {
	for line := range strings.SplitSeq(string(out), "\n") {
		clean := strings.TrimSpace(printable(line))
		if clean != "" {
			return clean
		}
	}
	return ""
}

func printable(s string) string {
	s = escapes.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if r == unicode.ReplacementChar || (unicode.IsControl(r) && r != '\t') {
			return -1
		}
		return r
	}, s)
}
