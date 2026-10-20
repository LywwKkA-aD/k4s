package logsink

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

// swapStderr points os.Stderr at a pipe until the returned func (or test
// cleanup) restores it; the func yields everything written meanwhile.
// Callers must not run in parallel: os.Stderr is process-wide.
func swapStderr(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&buf, r)
	}()

	var (
		once sync.Once
		out  string
	)
	stop := func() string {
		once.Do(func() {
			os.Stderr = orig
			_ = w.Close()
			<-done
			_ = r.Close()
			out = buf.String()
		})
		return out
	}
	t.Cleanup(func() { stop() })
	return stop
}
