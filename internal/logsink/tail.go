package logsink

import "sync"

// tailBuffer keeps the last limit bytes written to it. It is safe for
// concurrent use.
type tailBuffer struct {
	limit int

	mu        sync.Mutex
	data      []byte
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

// Write appends p, dropping the oldest bytes beyond the limit. It never
// fails.
func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	total := len(b.data) + len(p)
	if total <= b.limit {
		b.data = append(b.data, p...)
		return len(p), nil
	}
	b.truncated = true
	next := make([]byte, 0, b.limit)
	if len(p) < b.limit {
		next = append(next, b.data[total-b.limit:]...)
	}
	next = append(next, p[max(0, len(p)-b.limit):]...)
	b.data = next
	return len(p), nil
}

// Snapshot returns a copy of the retained bytes and whether older bytes
// were dropped.
func (b *tailBuffer) Snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.truncated
}
