package k8s

import (
	"bytes"
	"sync"
)

// ringSize bounds what a session keeps of the forwarder's error output: a
// forward that runs for days and fails a connection now and then must not
// grow without end.
const ringSize = 4 << 10

// ring is an io.Writer that keeps the last ringSize bytes written to it.
// It is safe for concurrent use.
type ring struct {
	mu  sync.Mutex
	buf []byte
	// cut is set once bytes were dropped from the front, which may have
	// left a partial line there.
	cut bool
}

// Write implements io.Writer. It never fails.
func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) >= ringSize {
		r.buf = append(r.buf[:0], p[len(p)-ringSize:]...)
		r.cut = true
		return len(p), nil
	}
	if over := len(r.buf) + len(p) - ringSize; over > 0 {
		r.buf = append(r.buf[:0], r.buf[over:]...)
		r.cut = true
	}
	r.buf = append(r.buf, p...)
	return len(p), nil
}

// String returns the whole lines the ring holds: a line cut short at the
// front is left out, unless it is all there is.
func (r *ring) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.buf
	if i := bytes.IndexByte(b, '\n'); r.cut && i >= 0 && i < len(b)-1 {
		b = b[i+1:]
	}
	return string(b)
}
