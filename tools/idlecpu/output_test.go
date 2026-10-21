package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestOutputCounter(t *testing.T) {
	t.Parallel()

	c := newOutputCounter()
	c.add([]byte("hello"))
	start := c.snapshot()
	c.add([]byte(" world"))
	c.add([]byte("!"))
	end := c.snapshot()

	if got := end.sub(start); got != (outputStats{bytes: 7, reads: 2}) {
		t.Fatalf("window output = %+v, want 7 bytes in 2 reads", got)
	}
	if got := string(c.lastOutput()); got != "hello world!" {
		t.Fatalf("tail = %q", got)
	}
}

func TestOutputCounterTailIsBounded(t *testing.T) {
	t.Parallel()

	c := newOutputCounter()
	c.add(bytes.Repeat([]byte("a"), 3*tailSize))
	c.add([]byte("end"))
	tail := c.lastOutput()
	if len(tail) != tailSize {
		t.Fatalf("tail holds %d bytes, want %d", len(tail), tailSize)
	}
	if !strings.HasSuffix(string(tail), "aend") {
		t.Fatalf("tail does not end with the latest output: %q", tail[len(tail)-8:])
	}
}

// chunkReader returns one chunk per Read, then its error.
type chunkReader struct {
	chunks []string
	err    error
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, r.err
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

func TestDrainCountsAndAnswers(t *testing.T) {
	t.Parallel()

	in := &chunkReader{chunks: []string{"frame\x1b]11;?", "\x07", "\x1b[c"}, err: io.EOF}
	var replies bytes.Buffer
	c := newOutputCounter()
	drain(in, &replies, c, newResponder(terminalQueries))

	if got := c.snapshot(); got.bytes != int64(len("frame\x1b]11;?\x07\x1b[c")) || got.reads != 3 {
		t.Fatalf("output = %+v", got)
	}
	if got, want := replies.String(), replyBackgroundBEL+replyDA1; got != want {
		t.Fatalf("replies = %q, want %q", got, want)
	}
}
