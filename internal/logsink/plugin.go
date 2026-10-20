package logsink

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// pluginStderrLimit bounds the in-memory copy of plugin stderr.
const pluginStderrLimit = 64 << 10

// drainTimeout bounds how long FlushPluginStderr waits for the pipe to
// empty. A plugin still running at exit (say, waiting on a browser login)
// holds the write end open, so EOF may never come.
const drainTimeout = 250 * time.Millisecond

// pluginStderr is process-wide because os.Stderr is.
var pluginStderr stderrCapture

// CapturePluginStderr runs build with os.Stderr pointed at an in-memory
// pipe once Install has run, and returns build's error. client-go's exec
// credential authenticator (EKS, GKE, OIDC plugins) keeps the os.Stderr it
// sees when a clientset is built and hands it to every later plugin run, so
// every clientset build must go through here. Plugin output (device codes,
// login URLs, sometimes tokens) then stays in memory: never on the TUI,
// never in the log file. Before Install it just runs build.
func CapturePluginStderr(build func() error) error {
	return pluginStderr.run(build)
}

// FlushPluginStderr stops the capture and writes what plugins printed (the
// last 64 KiB) to w, which should be the real stderr once the TUI has
// released the terminal. It writes nothing when plugins printed nothing.
func FlushPluginStderr(w io.Writer) error {
	return pluginStderr.flush(w, drainTimeout)
}

// stderrCapture owns one pipe for the life of the process: authenticators
// built under it keep writing to it long after the swap is undone.
type stderrCapture struct {
	mu      sync.Mutex // serialises swaps of os.Stderr
	enabled bool
	w       *os.File
	buf     *tailBuffer
	done    chan struct{}
}

func (c *stderrCapture) enable() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = true
}

// run swaps os.Stderr for the pipe around build. The mutex only orders
// k4s's own swaps; build must not race other code that writes os.Stderr.
func (c *stderrCapture) run(build func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabled {
		return build()
	}
	if c.w == nil {
		if err := c.startLocked(); err != nil {
			klog.ErrorS(err, "exec plugin stderr not captured")
			return build()
		}
	}
	orig := os.Stderr
	os.Stderr = c.w
	defer func() { os.Stderr = orig }()
	return build()
}

func (c *stderrCapture) startLocked() error {
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}
	buf := newTailBuffer(pluginStderrLimit)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(buf, r)
		_ = r.Close()
	}()
	c.w, c.buf, c.done = w, buf, done
	return nil
}

func (c *stderrCapture) flush(w io.Writer, wait time.Duration) error {
	c.mu.Lock()
	pw, buf, done := c.w, c.buf, c.done
	c.enabled, c.w, c.buf, c.done = false, nil, nil, nil
	c.mu.Unlock()
	if pw == nil {
		return nil
	}

	_ = pw.Close()
	select {
	case <-done:
	case <-time.After(wait):
	}

	data, truncated := buf.Snapshot()
	if len(data) == 0 {
		return nil
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	header := "k4s: exec credential plugin output while k4s was running:\n"
	if truncated {
		header = "k4s: exec credential plugin output while k4s was running (last 64 KiB):\n"
	}
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("write plugin stderr: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write plugin stderr: %w", err)
	}
	return nil
}
