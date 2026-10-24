package logsink

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestOpenAtWritesAndCounts(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := filepath.Join(home, ".local", "state", "k4s", "k4s.log")
	s := OpenAt(path, home)
	t.Cleanup(func() { _ = s.Close() })

	if s.Path() != path {
		t.Fatalf("Path() = %q, want %q", s.Path(), path)
	}
	for _, msg := range []string{"first\n", "", "second\nwith a second line\n"} {
		if _, err := fmt.Fprint(s, msg); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if got := s.Messages(); got != 2 {
		t.Errorf("Messages() = %d, want 2 (empty writes do not count, one write is one message)", got)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first\nsecond\nwith a second line\n" {
		t.Fatalf("log = %q", got)
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()

	shown := "~" + string(filepath.Separator) + filepath.Join(".local", "state", "k4s", "k4s.log")

	cases := []struct {
		name   string
		writes int
		want   string
	}{
		{name: "nothing logged", writes: 0, want: ""},
		{name: "one message", writes: 1, want: "k4s: 1 message logged to " + shown},
		{name: "many messages", writes: 3, want: "k4s: 3 messages logged to " + shown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			s := OpenAt(filepath.Join(home, ".local", "state", "k4s", "k4s.log"), home)
			t.Cleanup(func() { _ = s.Close() })
			for range tc.writes {
				_, _ = s.Write([]byte("msg\n"))
			}
			if got := s.Summary(); got != tc.want {
				t.Fatalf("Summary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenAtFallsBackToDiscard(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
	}{
		{name: "unopenable path", path: filepath.Join(blocker, "k4s", "k4s.log")},
		{name: "no path resolved", path: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := OpenAt(tc.path, "")
			t.Cleanup(func() { _ = s.Close() })

			if s.Path() != "" {
				t.Errorf("Path() = %q, want empty when discarding", s.Path())
			}
			if n, err := s.Write([]byte("lost\n")); err != nil || n != 5 {
				t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
			}
			got := s.Summary()
			if !strings.HasPrefix(got, "k4s: 1 message dropped") {
				t.Fatalf("Summary() = %q, want a dropped notice", got)
			}
		})
	}
}

func TestCloseIsIdempotentAndStopsCounting(t *testing.T) {
	t.Parallel()

	s := OpenAt(filepath.Join(t.TempDir(), "k4s.log"), "")
	_, _ = s.Write([]byte("kept\n"))
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if n, err := s.Write([]byte("late\n")); err != nil || n != 5 {
		t.Fatalf("Write after Close = (%d, %v), want (5, nil)", n, err)
	}
	if got := s.Messages(); got != 1 {
		t.Fatalf("Messages() = %d, want 1", got)
	}
}

func TestWriteIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	s := OpenAt(filepath.Join(t.TempDir(), "k4s.log"), "")
	t.Cleanup(func() { _ = s.Close() })

	const writers, each = 8, 50
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range each {
				_, _ = s.Write([]byte("line\n"))
			}
		})
	}
	wg.Wait()
	if got := s.Messages(); got != writers*each {
		t.Fatalf("Messages() = %d, want %d", got, writers*each)
	}
}

// Open reads the real environment, so it cannot run in parallel.
func TestOpenHonoursXDGStateHome(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	s := Open()
	t.Cleanup(func() { _ = s.Close() })

	want := filepath.Join(state, "k4s", "k4s.log")
	if s.Path() != want {
		t.Fatalf("Path() = %q, want %q", s.Path(), want)
	}
}

// A later `cat k4s.log` must not run what the cluster wrote: klog lines,
// API warnings and HandleError text quote object names and server text.
func TestWriteLogsInertText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain lines are kept", in: "I1024 first\n\tsecond\n", want: "I1024 first\n\tsecond\n"},
		{name: "OSC 52 clipboard write", in: "W pod \x1b]52;c;aGk=\x07web\n", want: "W pod web\n"},
		{name: "clear screen and title", in: "\x1b[2J\x1b]0;pwned\x1b\\E boom\n", want: "E boom\n"},
		{name: "bidi and zero-width characters", in: "Warning: \u202eweb\u202c w\u200bx\U000e0041\n", want: "Warning: web wx\n"},
		{name: "CRLF becomes LF", in: "a\r\nb\r\n", want: "a\nb\n"},
		{name: "sequence ends at its line", in: "a\x1b]0;t\nb\n", want: "a\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "k4s.log")
			s := OpenAt(path, "")
			n, err := s.Write([]byte(tc.in))
			if err != nil || n != len(tc.in) {
				t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(tc.in))
			}
			if err := s.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("log = %q, want %q", got, tc.want)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestInertWriterReportsWriteErrors(t *testing.T) {
	t.Parallel()

	n, err := inertWriter{w: failingWriter{}}.Write([]byte("line\n"))
	if err == nil || n != 0 {
		t.Fatalf("Write = (%d, %v), want (0, disk full)", n, err)
	}
}
