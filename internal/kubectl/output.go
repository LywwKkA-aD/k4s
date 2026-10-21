package kubectl

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
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

// firstLine returns the first non-blank line of out as inert text: the
// message comes from the cluster and can carry terminal escapes into the
// status line.
func firstLine(out []byte) string {
	for line := range strings.SplitSeq(string(out), "\n") {
		clean := strings.TrimSpace(printable(line))
		if clean != "" {
			return clean
		}
	}
	return ""
}

// printable drops the replacement runes the sanitizer puts in for invalid
// UTF-8: a status line is better without them.
func printable(s string) string {
	return strings.ReplaceAll(sanitize.Line(s), string(utf8.RuneError), "")
}
