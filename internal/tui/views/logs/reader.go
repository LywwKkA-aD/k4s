package logs

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// readBufferSize is the reader's buffer. A longer line is read in pieces,
// so its length costs no more memory than sanitize.MaxLine.
const readBufferSize = 32 << 10

// lineReader reads log lines of any length. It keeps the first
// sanitize.MaxLine bytes of a line and only counts the rest, so a 2 MiB
// line neither ends the stream (bufio.Scanner gives up past its buffer)
// nor holds 2 MiB.
type lineReader struct {
	r *bufio.Reader
}

func newLineReader(r io.Reader) lineReader {
	return lineReader{r: bufio.NewReaderSize(r, readBufferSize)}
}

// rawLine is one line as read: its first bytes without the line feed, and
// how many bytes after them were dropped. ok is false when nothing was
// read.
type rawLine struct {
	head    []byte
	dropped int
	ok      bool
}

// next returns the next line. At the end of the stream the error is
// io.EOF, together with a last line that had no line feed.
func (l lineReader) next() (rawLine, error) {
	var line rawLine
	for {
		chunk, err := l.r.ReadSlice('\n')
		line = line.add(chunk)
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err == nil:
			return line.withoutLF(), nil
		}
		return line, err
	}
}

// add appends chunk, keeping at most sanitize.MaxLine bytes. head is
// always a copy: chunk points into the reader's buffer.
func (l rawLine) add(chunk []byte) rawLine {
	if len(chunk) == 0 {
		return l
	}
	take := min(max(sanitize.MaxLine-len(l.head), 0), len(chunk))
	return rawLine{
		head:    append(l.head[:len(l.head):len(l.head)], chunk[:take]...),
		dropped: l.dropped + len(chunk) - take,
		ok:      true,
	}
}

// withoutLF drops the line feed that ended the line, which is the last
// byte read: in head unless the line overflowed.
func (l rawLine) withoutLF() rawLine {
	if l.dropped > 0 {
		l.dropped--
		return l
	}
	l.head = l.head[:len(l.head)-1]
	return l
}

// withoutTimestamp splits off the RFC3339Nano time the API server puts in
// front of a line asked for with timestamps; a line without one comes
// back whole with the zero time. The stamp is cut before the line is
// sanitized, so an escape right after it cannot hide it.
func (l rawLine) withoutTimestamp() (time.Time, rawLine) {
	i := bytes.IndexByte(l.head, ' ')
	if i <= 0 {
		return time.Time{}, l
	}
	ts, err := time.Parse(time.RFC3339Nano, string(l.head[:i]))
	if err != nil {
		return time.Time{}, l
	}
	l.head = l.head[i+1:]
	return ts, l
}

// text is the line made inert (see sanitize.Line), with "…[+N bytes]"
// for what was dropped. A cut that split a character moves the whole
// character to the dropped count.
func (l rawLine) text() string {
	head, dropped := l.head, l.dropped
	if dropped > 0 {
		head, dropped = wholeRunes(head, dropped)
	}
	s := sanitize.Line(string(head))
	if dropped == 0 {
		return s
	}
	return s + "…[+" + strconv.Itoa(dropped) + " bytes]"
}

// wholeRunes trims a character cut short at the end of head.
func wholeRunes(head []byte, dropped int) ([]byte, int) {
	for i := len(head) - 1; i >= 0 && i >= len(head)-utf8.UTFMax; i-- {
		if !utf8.RuneStart(head[i]) {
			continue
		}
		if utf8.FullRune(head[i:]) {
			return head, dropped
		}
		return head[:i], dropped + len(head) - i
	}
	return head, dropped
}
