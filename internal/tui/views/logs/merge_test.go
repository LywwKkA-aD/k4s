package logs

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

type fakeStreamer map[string]string

func (f fakeStreamer) StreamLogs(_ context.Context, _, podName string, _ k8s.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f[podName])), nil
}

func (f fakeStreamer) ContainersForPod(ctx context.Context, namespace, name string) ([]string, error) {
	return nil, nil
}

// recordingStreamer remembers which container each pod was streamed with, so
// tests can assert the per-pod fallback behaviour.
type recordingStreamer struct {
	logs       map[string]string
	containers map[string][]string
	mu         sync.Mutex
	used       map[string]string
}

func (r *recordingStreamer) StreamLogs(_ context.Context, _, podName string, opts k8s.LogOptions) (io.ReadCloser, error) {
	r.mu.Lock()
	r.used[podName] = opts.Container
	r.mu.Unlock()
	return io.NopCloser(strings.NewReader(r.logs[podName])), nil
}

func (r *recordingStreamer) ContainersForPod(ctx context.Context, namespace, name string) ([]string, error) {
	return r.containers[name], nil
}

func TestRunMergedStreamsFallsBackToPodContainer(t *testing.T) {
	t.Parallel()

	client := &recordingStreamer{
		logs: map[string]string{
			"pod-a": "2020-01-01T00:00:01.000000000Z a-1\n",
			"pod-b": "2020-01-01T00:00:02.000000000Z b-1\n",
		},
		containers: map[string][]string{
			"pod-a": {"app"},
			"pod-b": {"worker"}, // no "app" container on this pod
		},
		used: map[string]string{},
	}

	events := make(chan logEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runMergedStreams(ctx, client, "ns", []string{"pod-a", "pod-b"}, 100, "app", events)
	for range events {
	}

	if client.used["pod-a"] != "app" {
		t.Errorf("pod-a container = %q, want %q", client.used["pod-a"], "app")
	}
	if client.used["pod-b"] != "worker" {
		t.Errorf("pod-b should fall back to its own container, got %q", client.used["pod-b"])
	}
}

func TestRunMergedStreamsOrdersHistoryByTimestamp(t *testing.T) {
	t.Parallel()

	client := fakeStreamer{
		"pod-a": "2026-08-24T12:00:01.000000000Z a-1\n2026-08-24T12:00:03.000000000Z a-2\n2026-08-24T12:00:05.000000000Z a-3\n",
		"pod-b": "2026-08-24T12:00:02.000000000Z b-1\n2026-08-24T12:00:04.000000000Z b-2\n2026-08-24T12:00:06.000000000Z b-3\n",
	}

	events := make(chan logEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runMergedStreams(ctx, client, "ns", []string{"pod-a", "pod-b"}, 100, "", events)

	var got []string
	for ev := range events {
		if ev.Line != "" {
			got = append(got, ev.Line)
		}
	}

	want := []string{"a-1", "b-1", "a-2", "b-2", "a-3", "b-3"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRunMergedStreamsSortsWhateverTheNodeClockSays(t *testing.T) {
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
			events := make(chan logEvent, 64)
			runMergedStreams(context.Background(), client, "ns", []string{"pod-a", "pod-b"}, 100, "", events)

			var got []string
			for ev := range events {
				if ev.Line != "" {
					got = append(got, ev.Line)
				}
			}
			if strings.Join(got, " ") != "a-1 b-1 a-2 b-2" {
				t.Fatalf("lines = %v, want them in time order", got)
			}
		})
	}
}

func TestRunMergedStreamsEmitsErrors(t *testing.T) {
	t.Parallel()

	client := fakeStreamer{
		"pod-a": "",
		"pod-b": "2020-01-01T00:00:01.000000000Z b-1\n",
	}

	events := make(chan logEvent, 64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runMergedStreams(ctx, client, "ns", []string{"pod-a", "pod-b"}, 100, "", events)

	var lines, errs, dones int
	for ev := range events {
		switch {
		case ev.Err != nil:
			errs++
		case ev.Done:
			dones++
		default:
			lines++
		}
	}

	if lines != 1 {
		t.Errorf("lines = %d, want 1", lines)
	}
	if dones != 2 {
		t.Errorf("done markers = %d, want 2", dones)
	}
	if errs != 0 {
		t.Errorf("unexpected errors: %d", errs)
	}
}

// openStreamer serves each pod's body, then keeps the stream open, as a
// followed log does, until the caller's context ends.
type openStreamer map[string]string

func (f openStreamer) StreamLogs(ctx context.Context, _, podName string, _ k8s.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(strings.NewReader(f[podName]), untilDone{ctx})), nil
}

func (openStreamer) ContainersForPod(context.Context, string, string) ([]string, error) {
	return nil, nil
}

type untilDone struct{ ctx context.Context }

func (u untilDone) Read([]byte) (int, error) {
	<-u.ctx.Done()
	return 0, io.EOF
}

func TestRunMergedStreamsFlushesAQuietPodsHistory(t *testing.T) {
	t.Parallel()

	client := openStreamer{
		"pod-a": "2026-08-24T12:00:02.000000000Z a-2\n2026-08-24T12:00:01.000000000Z a-1\n",
		"pod-b": "",
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan logEvent, 64)
	start := time.Now()
	go runMergedStreams(ctx, client, "ns", []string{"pod-a", "pod-b"}, 100, "", events)

	var got []string
	for len(got) < 2 {
		select {
		case ev := <-events:
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
	cancel()
	for range events {
	}
}
