package main

import (
	"io"
	"slices"
	"sync"
	"sync/atomic"
)

// tailSize is how much of the latest output is kept to explain an early exit.
const tailSize = 512

// outputStats counts what the child wrote to the pty. Each read is
// roughly one renderer flush, so reads per second stand in for repaints.
type outputStats struct {
	bytes int64
	reads int64
}

func (s outputStats) sub(earlier outputStats) outputStats {
	return outputStats{bytes: s.bytes - earlier.bytes, reads: s.reads - earlier.reads}
}

// outputCounter is written by the drain goroutine and read by the sampler.
type outputCounter struct {
	bytes atomic.Int64
	reads atomic.Int64

	mu   sync.Mutex
	tail []byte
}

func newOutputCounter() *outputCounter { return &outputCounter{} }

func (c *outputCounter) add(p []byte) {
	c.bytes.Add(int64(len(p)))
	c.reads.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(p) >= tailSize {
		c.tail = slices.Clone(p[len(p)-tailSize:])
		return
	}
	joined := append(slices.Clone(c.tail), p...)
	c.tail = joined[max(0, len(joined)-tailSize):]
}

func (c *outputCounter) snapshot() outputStats {
	return outputStats{bytes: c.bytes.Load(), reads: c.reads.Load()}
}

func (c *outputCounter) lastOutput() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.tail)
}

// drain reads the pty until it fails, which it does once the child and
// everything holding the terminal have exited, and answers terminal queries.
func drain(r io.Reader, replies io.Writer, c *outputCounter, resp responder) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.add(buf[:n])
			var reply []byte
			resp, reply = resp.scan(buf[:n])
			if len(reply) > 0 {
				// A failed reply means the child is gone; the next Read says so.
				_, _ = replies.Write(reply)
			}
		}
		if err != nil {
			return
		}
	}
}
