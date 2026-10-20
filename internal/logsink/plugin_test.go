package logsink

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests swap os.Stderr and therefore do not run in parallel.

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
	if err := c.flush(&out, time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("flush wrote %q with nothing captured", out.String())
	}
}

func TestCaptureHoldsStderrWrittenDuringAndAfterBuild(t *testing.T) {
	stop := swapStderr(t)
	c := &stderrCapture{}
	c.enable()

	// held mimics client-go's exec authenticator: it keeps the os.Stderr it
	// saw at construction and writes to it later.
	var held *os.File
	if err := c.run(func() error {
		held = os.Stderr
		_, _ = fmt.Fprintln(os.Stderr, "during build")
		return nil
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := fmt.Fprintln(held, "device code ABCD-EFGH"); err != nil {
		t.Fatalf("write to held stderr: %v", err)
	}

	// A second build reuses the same pipe.
	if err := c.run(func() error {
		if os.Stderr != held {
			t.Error("second build got a different capture pipe")
		}
		return nil
	}); err != nil {
		t.Fatalf("second run: %v", err)
	}

	var out bytes.Buffer
	if err := c.flush(&out, time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}
	terminal := stop()

	got := out.String()
	for _, want := range []string{"during build", "device code ABCD-EFGH"} {
		if !strings.Contains(got, want) {
			t.Errorf("flushed output %q missing %q", got, want)
		}
		if strings.Contains(terminal, want) {
			t.Errorf("%q leaked to the terminal: %q", want, terminal)
		}
	}
	if !strings.HasPrefix(got, "k4s: ") {
		t.Errorf("flushed output %q lacks the k4s header", got)
	}
}

func TestCaptureRestoresStderrOnPanic(t *testing.T) {
	swapStderr(t)
	before := os.Stderr

	c := &stderrCapture{}
	c.enable()
	t.Cleanup(func() { _ = c.flush(&bytes.Buffer{}, time.Second) })

	func() {
		defer func() { _ = recover() }()
		_ = c.run(func() error { panic("boom") })
	}()
	if os.Stderr != before {
		t.Fatal("os.Stderr not restored after a panicking build")
	}
}

func TestFlushStopsCapture(t *testing.T) {
	c := &stderrCapture{}
	c.enable()
	_ = c.run(func() error { return nil })
	if err := c.flush(&bytes.Buffer{}, time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}

	orig := os.Stderr
	_ = c.run(func() error {
		if os.Stderr != orig {
			t.Error("capture still active after flush")
		}
		return nil
	})
}

func TestFlushMarksTruncatedOutput(t *testing.T) {
	swapStderr(t)

	c := &stderrCapture{}
	c.enable()
	if err := c.run(func() error {
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("x"), pluginStderrLimit+10))
		return nil
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out bytes.Buffer
	if err := c.flush(&out, time.Second); err != nil {
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
