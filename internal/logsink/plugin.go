package logsink

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"k8s.io/klog/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// pluginStderrLimit bounds the in-memory copy of plugin stderr.
const pluginStderrLimit = 64 << 10

// pluginStderr is process-wide because os.Stderr is.
var pluginStderr stderrCapture

// CapturePluginStderr runs build with os.Stderr pointed at a pipe once
// Install has run, and returns build's error. client-go's exec credential
// authenticator (EKS, GKE, OIDC plugins) keeps the os.Stderr it sees when a
// clientset is built and hands it to every later plugin run, so every
// clientset build must go through here. Until HoldPluginStderr is called
// the pipe passes plugin output straight to the real stderr, so login
// prompts before the TUI starts stay visible. Before Install it just runs
// build.
func CapturePluginStderr(build func() error) error {
	return pluginStderr.run(build)
}

// HoldPluginStderr makes plugin output stay in memory (the last 64 KiB)
// instead of reaching the terminal. Call it right before the TUI takes the
// terminal: plugin output (device codes, login URLs, sometimes tokens) then
// never paints over the screen and never lands in the log file.
func HoldPluginStderr() {
	pluginStderr.hold()
}

// FlushPluginStderr writes what plugins printed while held to w as inert
// text (w should be the real stderr once the TUI has released the
// terminal), and lets later plugin output through to the real stderr
// again. It writes nothing when plugins printed nothing. The pipe stays
// open: cached authenticators keep writing to it.
func FlushPluginStderr(w io.Writer) error {
	return pluginStderr.flush(w)
}

// stderrCapture owns one pipe for the life of the process: authenticators
// built under it keep writing to it long after the swap is undone.
type stderrCapture struct {
	mu      sync.Mutex // serialises swaps of os.Stderr
	enabled bool
	w       *os.File
	route   *routeWriter
	holding bool // applied to route once the pipe exists
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

// startLocked creates the pipe. The current os.Stderr is the real terminal
// stderr at this point, so it becomes the pass-through target.
func (c *stderrCapture) startLocked() error {
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}
	route := newRouteWriter(os.Stderr)
	route.setHold(c.holding)
	go func() {
		_, _ = io.Copy(route, r)
		_ = r.Close()
	}()
	c.w, c.route = w, route
	return nil
}

func (c *stderrCapture) hold() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.holding = true
	if c.route != nil {
		c.route.setHold(true)
	}
}

func (c *stderrCapture) flush(w io.Writer) error {
	c.mu.Lock()
	c.holding = false
	route := c.route
	c.mu.Unlock()
	if route == nil {
		return nil
	}

	report := heldReport(route.release())
	if report == "" {
		return nil
	}
	if _, err := io.WriteString(w, report); err != nil {
		return fmt.Errorf("write plugin stderr: %w", err)
	}
	return nil
}

// heldReport is what flush prints for held plugin output: a header, then
// the output as inert text (sanitize.Text), since it reaches the real
// terminal once k4s has given it back. Nothing held, or nothing left once
// inert, prints nothing.
func heldReport(data []byte, truncated bool) string {
	text := sanitize.Text(string(data))
	if text == "" {
		return ""
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	header := "k4s: exec credential plugin output while k4s was running:\n"
	if truncated {
		header = "k4s: exec credential plugin output while k4s was running (last 64 KiB):\n"
	}
	return header + text
}

// routeWriter receives everything plugins write to the pipe and sends it
// either to the real stderr or, while held, to a tail buffer.
type routeWriter struct {
	out io.Writer

	mu   sync.Mutex
	hold bool
	buf  *tailBuffer
}

func newRouteWriter(out io.Writer) *routeWriter {
	return &routeWriter{out: out, buf: newTailBuffer(pluginStderrLimit)}
}

// Write never fails: the pipe must keep draining or plugins block.
func (r *routeWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hold {
		return r.buf.Write(p)
	}
	_, _ = r.out.Write(p)
	return len(p), nil
}

func (r *routeWriter) setHold(hold bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hold = hold
}

// release stops holding and returns what was held, starting a fresh buffer.
func (r *routeWriter) release() ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, truncated := r.buf.Snapshot()
	r.hold = false
	r.buf = newTailBuffer(pluginStderrLimit)
	return data, truncated
}
