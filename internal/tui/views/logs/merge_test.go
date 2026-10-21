package logs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

type fakeStreamer map[string]string

func (f fakeStreamer) StreamLogs(_ context.Context, _, podName string, _ k8s.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f[podName])), nil
}

// merged runs a merged session over src, one reader a pod, and returns
// its events up to every reader's end. The session stays open, as the
// view's does, until the test ends.
func merged(t *testing.T, src streamer, pods ...string) []logEvent {
	t.Helper()
	s := startSession(async.New(), src, "ns", defaultCrashLog(), pumpMerged, pods, 100)
	t.Cleanup(s.stop)
	for i, pod := range pods {
		s.attach(attachReq{pod: pod, attempt: uint64(i + 1), opts: k8s.LogOptions{Tail: 100}})
	}
	var out []logEvent
	timeout := time.After(5 * time.Second)
	for ends := 0; ends < len(pods); {
		select {
		case ev := <-s.events:
			out = append(out, ev)
			if ev.Done {
				ends++
			}
		case <-timeout:
			t.Fatalf("the streams never ended, got %+v", out)
		}
	}
	return out
}

func lineTexts(evs []logEvent) []string {
	var out []string
	for _, ev := range evs {
		if !ev.Done {
			out = append(out, ev.Line)
		}
	}
	return out
}

func TestMergeOrdersHistoryByTimestamp(t *testing.T) {
	t.Parallel()

	client := fakeStreamer{
		"pod-a": "2026-08-24T12:00:01.000000000Z a-1\n2026-08-24T12:00:03.000000000Z a-2\n2026-08-24T12:00:05.000000000Z a-3\n",
		"pod-b": "2026-08-24T12:00:02.000000000Z b-1\n2026-08-24T12:00:04.000000000Z b-2\n2026-08-24T12:00:06.000000000Z b-3\n",
	}
	got := lineTexts(merged(t, client, "pod-a", "pod-b"))
	if want := "a-1 b-1 a-2 b-2 a-3 b-3"; strings.Join(got, " ") != want {
		t.Fatalf("lines = %v, want %s", got, want)
	}
}

func TestMergeSortsWhateverTheNodeClockSays(t *testing.T) {
	t.Parallel()

	// The old merge took a line stamped after the stream opened as live
	// and let it through unsorted: a node clock ahead of the laptop's
	// scrambled the whole history.
	for _, skew := range []time.Duration{-10 * time.Minute, 10 * time.Minute} {
		t.Run(skew.String(), func(t *testing.T) {
			t.Parallel()
			node := time.Now().Add(skew)
			stamp := func(d time.Duration, text string) string {
				return node.Add(d).UTC().Format(time.RFC3339Nano) + " " + text + "\n"
			}
			client := fakeStreamer{
				"pod-a": stamp(1*time.Second, "a-1") + stamp(3*time.Second, "a-2"),
				"pod-b": stamp(2*time.Second, "b-1") + stamp(4*time.Second, "b-2"),
			}
			if got := lineTexts(merged(t, client, "pod-a", "pod-b")); strings.Join(got, " ") != "a-1 b-1 a-2 b-2" {
				t.Fatalf("lines = %v, want them in time order", got)
			}
		})
	}
}

func TestMergeSendsEachReadersEnd(t *testing.T) {
	t.Parallel()

	client := fakeStreamer{"pod-a": "", "pod-b": "2020-01-01T00:00:01.000000000Z b-1\n"}
	evs := merged(t, client, "pod-a", "pod-b")
	var lines, errs int
	ends := map[string]uint64{}
	for _, ev := range evs {
		switch {
		case ev.Err != nil:
			errs++
		case ev.Done:
			ends[ev.Pod] = ev.Attempt
		default:
			lines++
			if !ev.TS.Equal(time.Date(2020, 1, 1, 0, 0, 1, 0, time.UTC)) || ev.Attempt != 2 {
				t.Errorf("line %+v lost its stamp or its reader", ev)
			}
		}
	}
	if lines != 1 || errs != 0 || ends["pod-a"] != 1 || ends["pod-b"] != 2 {
		t.Fatalf("lines %d, errors %d, ends %v", lines, errs, ends)
	}
}

// openStreamer serves each pod's body, then keeps the stream open, as a
// followed log does, until the caller's context ends.
type openStreamer map[string]string

func (f openStreamer) StreamLogs(ctx context.Context, _, podName string, _ k8s.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(strings.NewReader(f[podName]), untilDone{ctx})), nil
}

type untilDone struct{ ctx context.Context }

func (u untilDone) Read([]byte) (int, error) {
	<-u.ctx.Done()
	return 0, io.EOF
}

func TestMergeFlushesAQuietPodsHistory(t *testing.T) {
	t.Parallel()

	client := openStreamer{
		"pod-a": "2026-08-24T12:00:02.000000000Z a-2\n2026-08-24T12:00:01.000000000Z a-1\n",
		"pod-b": "",
	}
	s := startSession(async.New(), client, "ns", defaultCrashLog(), pumpMerged, []string{"pod-a", "pod-b"}, 100)
	t.Cleanup(s.stop)
	start := time.Now()
	s.attach(attachReq{pod: "pod-a", attempt: 1})
	s.attach(attachReq{pod: "pod-b", attempt: 2})

	var got []string
	for len(got) < 2 {
		select {
		case ev := <-s.events:
			got = append(got, ev.Line)
		case <-time.After(5 * time.Second):
			t.Fatalf("history never went out, got %v", got)
		}
	}
	// historyIdle, with room for a loaded race-detector run, and well under
	// the old 1.5 s idle timeout.
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("history waited %v for streams that stay open", waited)
	}
	if strings.Join(got, " ") != "a-1 a-2" {
		t.Errorf("history = %v, want it in time order", got)
	}
}

// A reconnect asks from the second its last line fell in; the lines up to
// that line's own stamp were shown and are dropped, the rest kept.
func TestReaderDropsWhatItHasShown(t *testing.T) {
	t.Parallel()

	after := time.Date(2026, 10, 21, 12, 0, 5, 500, time.UTC)
	body := "2026-10-21T12:00:05.000000400Z shown\n" +
		"2026-10-21T12:00:05.000000500Z shown too\n" +
		"2026-10-21T12:00:05.000000600Z new\n" +
		"no stamp\n"
	out := make(chan parsedEvent, 8)
	readOpen(context.Background(), io.NopCloser(strings.NewReader(body)), attachReq{pod: "p", attempt: 3, after: after}, out)
	close(out)
	var got []string
	for ev := range out {
		if ev.attempt != 3 {
			t.Errorf("event of attempt %d", ev.attempt)
		}
		got = append(got, ev.text)
	}
	if want := "new|no stamp|"; strings.Join(got, "|") != want {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// cutStream serves body, then fails as a dropped connection does.
type cutStream struct{ io.Reader }

func (c cutStream) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if err == io.EOF {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func TestReaderDropsALineCutShort(t *testing.T) {
	t.Parallel()

	body := "2026-10-21T12:00:01Z whole\n2026-10-21T12:00:02Z cut sh"
	out := make(chan parsedEvent, 8)
	readOpen(context.Background(), io.NopCloser(cutStream{strings.NewReader(body)}), attachReq{pod: "p"}, out)
	close(out)
	var got []string
	var ended error
	for ev := range out {
		if ev.kind == lineStreamErr {
			ended = ev.err
			continue
		}
		got = append(got, ev.text)
	}
	if strings.Join(got, "|") != "whole" || !errors.Is(ended, io.ErrUnexpectedEOF) {
		t.Fatalf("lines = %q, end %v", got, ended)
	}
}
