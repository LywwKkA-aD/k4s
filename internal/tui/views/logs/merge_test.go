package logs

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

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

func TestRunMergedStreamsMarksLiveLinesAfterCutoff(t *testing.T) {
	t.Parallel()

	// These lines are far in the past, so they are historical. We rely on the
	// scanner finishing before the idle timeout, which means all events become
	// history and the live phase sees nothing.
	client := fakeStreamer{
		"pod-a": "2020-01-01T00:00:01.000000000Z old-a\n",
		"pod-b": "2020-01-01T00:00:02.000000000Z old-b\n",
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

	want := []string{"old-a", "old-b"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
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
