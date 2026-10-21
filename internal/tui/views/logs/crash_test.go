package logs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// syncBuffer is a log destination the stream goroutines may share.
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

// panicky streams the pods in bodies; the pods in boom panic on the first
// read, as a bug in k4s's reader would.
type panicky struct {
	bodies map[string]string
	boom   map[string]bool
}

func (p panicky) StreamLogs(_ context.Context, _, pod string, _ k8s.LogOptions) (io.ReadCloser, error) {
	if p.boom[pod] {
		return io.NopCloser(boomReader{}), nil
	}
	return io.NopCloser(strings.NewReader(p.bodies[pod])), nil
}

type boomReader struct{}

func (boomReader) Read([]byte) (int, error) { panic("reader bug") }

func crashed(t *testing.T, pods []string, src streamer) (Model, string, *syncBuffer) {
	t.Helper()
	logged := &syncBuffer{}
	m := New(nil, "demo", pods, 100, "", withSource(src), withCrashLog(crashLog{logger: log.New(logged, "", 0)}))
	t.Cleanup(func() { _ = m.Close() })
	m, out := tuitest.Mount(t, m, 100, 20)
	st, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || st.Level != status.Error {
		t.Fatalf("status = %+v (found %v), want an error", st, ok)
	}
	return m, st.Text, logged
}

func TestPanicInAStreamBecomesAnError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		pods  []string
		src   panicky
		shows []string
	}{
		{
			name:  "single pod",
			pods:  []string{"web-1"},
			src:   panicky{boom: map[string]bool{"web-1": true}},
			shows: []string{"[web-1] ○ ended: internal error (see the k4s log)"},
		},
		{
			name:  "one reader of a merge",
			pods:  []string{"web-1", "web-2"},
			src:   panicky{bodies: map[string]string{"web-2": "2026-08-24T12:00:01Z still here\n"}, boom: map[string]bool{"web-1": true}},
			shows: []string{"[web-1] ○ ended: internal error (see the k4s log)", "still here"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, text, logged := crashed(t, tc.pods, tc.src)
			if text != "internal error (see the k4s log)" {
				t.Errorf("status = %q", text)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Press("t"))
			screen := tuitest.Screen(m)
			for _, s := range tc.shows {
				if !strings.Contains(screen, s) {
					t.Errorf("screen lacks %q:\n%s", s, screen)
				}
			}
			if got := logged.String(); !strings.Contains(got, "reader bug") || !strings.Contains(got, "goroutine") {
				t.Errorf("the log lacks the panic and its stack:\n%s", got)
			}
		})
	}
}

func TestPanicInTheMergeLoopEndsTheMerge(t *testing.T) {
	t.Parallel()

	logged := &syncBuffer{}
	crash := crashLog{logger: log.New(logged, "", 0)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := newSession(ctx, cancel, async.New(), nil, "demo", crash, 1)
	s.incoming <- parsedEvent{pod: "web-1", text: "x", kind: lineLog}
	// A merger without its pods map panics on the first event.
	s.merge(&merger{})

	ev, ok := <-s.events
	var internal internalError
	if !ok || !errors.As(ev.Err, &internal) {
		t.Fatalf("event = %+v (open %v), want the internal error", ev, ok)
	}
	if !strings.Contains(logged.String(), "assignment to entry in nil map") {
		t.Errorf("the log lacks the panic:\n%s", logged.String())
	}
}

func TestCrashLogNamesTheK4sLog(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "k4s.log")
	sink := logsink.OpenAt(path, "")
	t.Cleanup(func() { _ = sink.Close() })
	err := crashLog{logger: log.New(sink, "", 0)}.recovered("a test", "boom")
	if got, want := err.Error(), "internal error (see "+path+")"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if sink.Messages() != 1 {
		t.Errorf("the sink got %d messages, want the stack as one", sink.Messages())
	}
	if got := (crashLog{logger: log.New(io.Discard, "", 0)}).recovered("a test", "boom").Error(); got != "internal error (see the k4s log)" {
		t.Errorf("without the sink: %q", got)
	}
}
