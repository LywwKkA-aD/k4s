package logsink

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests swap os.Stderr and therefore do not run in parallel.

// syncBuffer is a bytes.Buffer safe for one writer and concurrent readers.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeTerminal points os.Stderr at a pipe and collects what reaches it.
func fakeTerminal(t *testing.T) *syncBuffer {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	term := &syncBuffer{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(term, r)
	}()
	t.Cleanup(func() {
		os.Stderr = orig
		_ = w.Close()
		<-done
		_ = r.Close()
	})
	return term
}

// eventually polls cond: plugin output crosses a pipe and a goroutine
// before it lands anywhere.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// buildHoldingStderr runs one capture build and returns the stderr it saw,
// the way client-go's exec authenticator keeps it for later plugin runs.
func buildHoldingStderr(t *testing.T, c *stderrCapture, during string) *os.File {
	t.Helper()
	var held *os.File
	if err := c.run(func() error {
		held = os.Stderr
		if during != "" {
			_, _ = fmt.Fprintln(os.Stderr, during)
		}
		return nil
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	return held
}

func heldBytes(c *stderrCapture) int {
	data, _ := c.route.buf.Snapshot()
	return len(data)
}

func TestCaptureRunsBuildUntouchedWhenDisabled(t *testing.T) {
	c := &stderrCapture{}
	orig := os.Stderr
	sentinel := errors.New("build failed")

	err := c.run(func() error {
		if os.Stderr != orig {
			t.Error("os.Stderr swapped while capture is disabled")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("run() = %v, want the build error", err)
	}

	var out bytes.Buffer
	if err := c.flush(&out); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("flush wrote %q with nothing captured", out.String())
	}
}

func TestCapturePassesThroughUntilHeld(t *testing.T) {
	term := fakeTerminal(t)
	c := &stderrCapture{}
	c.enable()

	held := buildHoldingStderr(t, c, "")
	if held == os.Stderr {
		t.Fatal("build saw the real stderr, not the capture pipe")
	}
	const prompt = "open https://login.example/device and enter ABCD-EFGH"
	_, _ = fmt.Fprintln(held, prompt)

	eventually(t, "the login prompt on the terminal", func() bool {
		return strings.Contains(term.String(), prompt)
	})
	var out bytes.Buffer
	if err := c.flush(&out); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("flush replayed %q that was already shown", out.String())
	}
}

func TestCaptureHoldsWhileTUIRuns(t *testing.T) {
	term := fakeTerminal(t)
	c := &stderrCapture{}
	c.enable()
	c.hold() // before the pipe exists: the first build must start held

	held := buildHoldingStderr(t, c, "during build")
	_, _ = fmt.Fprintln(held, "device code ABCD-EFGH")

	// A second build reuses the same pipe.
	if err := c.run(func() error {
		if os.Stderr != held {
			t.Error("second build got a different capture pipe")
		}
		return nil
	}); err != nil {
		t.Fatalf("second run: %v", err)
	}

	want := len("during build\ndevice code ABCD-EFGH\n")
	eventually(t, "held plugin output", func() bool { return heldBytes(c) == want })

	var out bytes.Buffer
	if err := c.flush(&out); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := out.String()
	for _, s := range []string{"during build", "device code ABCD-EFGH"} {
		if !strings.Contains(got, s) {
			t.Errorf("flushed output %q missing %q", got, s)
		}
		if strings.Contains(term.String(), s) {
			t.Errorf("%q reached the terminal while held: %q", s, term.String())
		}
	}
	if !strings.HasPrefix(got, "k4s: ") {
		t.Errorf("flushed output %q lacks the k4s header", got)
	}
}

func TestFlushLetsLaterOutputThrough(t *testing.T) {
	term := fakeTerminal(t)
	c := &stderrCapture{}
	c.enable()
	c.hold()
	held := buildHoldingStderr(t, c, "")

	if err := c.flush(&bytes.Buffer{}); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// A token refresh after the TUI exited: the pipe is still open and the
	// output goes straight to the terminal.
	if _, err := fmt.Fprintln(held, "refreshed after exit"); err != nil {
		t.Fatalf("write after flush: %v", err)
	}
	eventually(t, "post-flush output on the terminal", func() bool {
		return strings.Contains(term.String(), "refreshed after exit")
	})
}

func TestCaptureRestoresStderrOnPanic(t *testing.T) {
	fakeTerminal(t)
	before := os.Stderr

	c := &stderrCapture{}
	c.enable()

	func() {
		defer func() { _ = recover() }()
		_ = c.run(func() error { panic("boom") })
	}()
	if os.Stderr != before {
		t.Fatal("os.Stderr not restored after a panicking build")
	}
}

func TestFlushMarksTruncatedOutput(t *testing.T) {
	fakeTerminal(t)

	c := &stderrCapture{}
	c.enable()
	c.hold()
	if err := c.run(func() error {
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("x"), pluginStderrLimit+10))
		return nil
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	eventually(t, "a full tail buffer", func() bool {
		_, truncated := c.route.buf.Snapshot()
		return truncated
	})

	var out bytes.Buffer
	if err := c.flush(&out); err != nil {
		t.Fatalf("flush: %v", err)
	}
	header, body, ok := strings.Cut(out.String(), "\n")
	if !ok || !strings.Contains(header, "last 64 KiB") {
		t.Fatalf("header %q should say the output was truncated", header)
	}
	if got := strings.TrimSuffix(body, "\n"); len(got) != pluginStderrLimit {
		t.Fatalf("body length = %d, want %d", len(got), pluginStderrLimit)
	}
}

// Plugin output is printed to the real terminal after k4s exits; a plugin,
// or the server it talks to, must not drive that terminal.
func TestHeldReportIsInert(t *testing.T) {
	t.Parallel()

	const header = "k4s: exec credential plugin output while k4s was running:\n"
	const cutHeader = "k4s: exec credential plugin output while k4s was running (last 64 KiB):\n"
	cases := []struct {
		name      string
		in        string
		truncated bool
		want      string
	}{
		{name: "nothing held", in: "", want: ""},
		{name: "plain lines", in: "open https://login.example/device\ncode ABCD-EFGH\n", want: header + "open https://login.example/device\ncode ABCD-EFGH\n"},
		{name: "a newline is added at the end", in: "code ABCD", want: header + "code ABCD\n"},
		{name: "OSC 52 clipboard write", in: "code \x1b]52;c;aGk=\x07ABCD\n", want: header + "code ABCD\n"},
		{name: "clear screen and colours", in: "\x1b[2J\x1b[31merror\x1b[0m: expired\n", want: header + "error: expired\n"},
		{name: "bidi and zero-width characters", in: "user \u202eadmin\u202c\u200b\n", want: header + "user admin\n"},
		{name: "CRLF becomes LF", in: "a\r\nb\r\n", want: header + "a\nb\n"},
		{name: "only escapes print nothing", in: "\x1b[2J\x1b]0;t\x07", want: ""},
		{name: "truncated output says so", in: "tail\n", truncated: true, want: cutHeader + "tail\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := heldReport([]byte(tc.in), tc.truncated); got != tc.want {
				t.Fatalf("heldReport(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFlushWritesInertText(t *testing.T) {
	fakeTerminal(t)
	c := &stderrCapture{}
	c.enable()
	c.hold()
	held := buildHoldingStderr(t, c, "")
	const raw = "code \x1b]52;c;aGk=\x07ABCD\x1b[2J\n"
	_, _ = io.WriteString(held, raw)
	eventually(t, "held plugin output", func() bool { return heldBytes(c) == len(raw) })

	var out bytes.Buffer
	if err := c.flush(&out); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := out.String(); strings.ContainsRune(got, '\x1b') || !strings.HasSuffix(got, "\ncode ABCD\n") {
		t.Fatalf("flushed %q, want the header and an inert \"code ABCD\"", got)
	}
}
