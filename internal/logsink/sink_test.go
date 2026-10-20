package logsink

import (
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
