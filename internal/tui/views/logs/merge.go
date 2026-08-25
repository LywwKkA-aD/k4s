package logs

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// Multi-pod log streaming is a two-phase merge:
//
//   1. History phase: each pod's tail burst is buffered and then emitted sorted
//      by timestamp. Without this the user sees all of pod A's history followed
//      by all of pod B's history because the per-pod goroutines race to the
//      shared channel.
//   2. Live phase: once every pod has crossed the historical cutoff, new lines
//      are forwarded as they arrive.
//
// The cutoff is per-stream: a line is historical when its timestamp is earlier
// than the moment the stream was opened. Streams that produce no history
// advance to the live phase implicitly after historyIdleTimeout.

const (
	historyIdleTimeout     = 1500 * time.Millisecond
	historyTimeTolerance   = 100 * time.Millisecond
	mergePerPodBuffer      = 64
	mergeIncomingBufferCap = 16
)

// streamer is the subset of *k8s.Client needed by the merge coordinator. It
// keeps tests free of a real Kubernetes clientset.
type streamer interface {
	StreamPodLogsWithTimestamps(ctx context.Context, namespace, podName string, tailLines int64, container string) (io.ReadCloser, error)
}

// parsedEvent is one item produced by a per-pod reader. It mirrors logEvent but
// carries the parsed timestamp used for ordering.
type parsedEvent struct {
	pod  string
	text string
	ts   time.Time
	kind lineKind
	live bool // true when the event belongs to the live phase
}

// runMergedStreams coordinates one reader per pod, merges the historical tail,
// then forwards live events. It closes events when all readers have finished or
// the context is cancelled.
func runMergedStreams(ctx context.Context, client streamer, namespace string, pods []string, tail int64, container string, events chan<- logEvent) {
	if len(pods) == 0 {
		close(events)
		return
	}

	perPod := make([]chan parsedEvent, len(pods))
	incoming := make(chan parsedEvent, len(pods)*mergeIncomingBufferCap)

	var streamWg, forwarderWg sync.WaitGroup
	for i, pod := range pods {
		perPod[i] = make(chan parsedEvent, mergePerPodBuffer)
		streamWg.Add(1)
		go func(p string, out chan<- parsedEvent) {
			defer streamWg.Done()
			streamOnePodMerged(ctx, client, namespace, p, tail, container, out)
			close(out)
		}(pod, perPod[i])

		forwarderWg.Add(1)
		go func(p string, in <-chan parsedEvent) {
			defer forwarderWg.Done()
			for ev := range in {
				ev.pod = p
				select {
				case incoming <- ev:
				case <-ctx.Done():
					return
				}
			}
		}(pod, perPod[i])
	}

	go func() {
		streamWg.Wait()
		forwarderWg.Wait()
		close(incoming)
	}()

	mergeEvents(ctx, pods, incoming, events)
}

// mergeEvents reads parsed events from all pods, emits the sorted history, then
// forwards live events. It also closes the outgoing events channel when done.
func mergeEvents(ctx context.Context, pods []string, incoming <-chan parsedEvent, events chan<- logEvent) {
	defer close(events)

	history, postHistory := collectHistory(ctx, pods, incoming)
	if ctx.Err() != nil {
		return
	}

	emitSortedEvents(ctx, events, history, postHistory)
	if ctx.Err() != nil {
		return
	}

	forwardLive(ctx, incoming, events)
}

// collectHistory buffers the per-pod tail burst until every pod has crossed
// the historical cutoff. It returns the historical lines and any live/error
// markers that arrived during the collection window.
func collectHistory(ctx context.Context, pods []string, incoming <-chan parsedEvent) ([]parsedEvent, []parsedEvent) {
	historyPending := len(pods)
	historyDone := make(map[string]bool, len(pods))
	history := make([]parsedEvent, 0, len(pods)*128)
	postHistory := make([]parsedEvent, 0, len(pods))

	historyDeadline := time.NewTimer(historyIdleTimeout)
	defer historyDeadline.Stop()

	for historyPending > 0 {
		select {
		case <-ctx.Done():
			return history, postHistory
		case <-historyDeadline.C:
			historyPending = 0
		case ev, ok := <-incoming:
			if !ok {
				historyPending = 0
				break
			}
			if !historyDone[ev.pod] && (ev.live || ev.kind != lineLog) {
				historyDone[ev.pod] = true
				historyPending--
			}
			if ev.kind == lineLog && !ev.live {
				history = append(history, ev)
			} else {
				postHistory = append(postHistory, ev)
			}
		}
	}
	return history, postHistory
}

// emitSortedEvents flushes the merged history (and any post-history markers)
// ordered by timestamp. Stable sort keeps per-pod arrival order when multiple
// lines share the same timestamp, so replica bursts stay readable.
func emitSortedEvents(ctx context.Context, events chan<- logEvent, history, postHistory []parsedEvent) {
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].ts.IsZero() {
			return true
		}
		if history[j].ts.IsZero() {
			return false
		}
		return history[i].ts.Before(history[j].ts)
	})
	for _, ev := range history {
		if !sendLogEvent(ctx, events, logEvent{Pod: ev.pod, Line: ev.text}) {
			return
		}
	}

	sort.SliceStable(postHistory, func(i, j int) bool {
		if postHistory[i].ts.IsZero() {
			return !postHistory[j].ts.IsZero()
		}
		if postHistory[j].ts.IsZero() {
			return false
		}
		return postHistory[i].ts.Before(postHistory[j].ts)
	})
	for _, ev := range postHistory {
		if !sendParsedAsLogEvent(ctx, events, ev) {
			return
		}
	}
}

// forwardLive passes through every subsequent event as it arrives.
func forwardLive(ctx context.Context, incoming <-chan parsedEvent, events chan<- logEvent) {
	for ev := range incoming {
		if ctx.Err() != nil {
			return
		}
		if !sendParsedAsLogEvent(ctx, events, ev) {
			return
		}
	}
}

func sendParsedAsLogEvent(ctx context.Context, events chan<- logEvent, ev parsedEvent) bool {
	var out logEvent
	switch ev.kind {
	case lineLog:
		out = logEvent{Pod: ev.pod, Line: ev.text}
	case lineStreamErr:
		out = logEvent{Pod: ev.pod, Err: errors.New(ev.text)}
	case lineStreamDone:
		out = logEvent{Pod: ev.pod, Done: true}
	}
	return sendLogEvent(ctx, events, out)
}

func sendLogEvent(ctx context.Context, events chan<- logEvent, ev logEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- ev:
		return true
	}
}

func streamOnePodMerged(ctx context.Context, client streamer, ns, pod string, tail int64, container string, out chan<- parsedEvent) {
	openTime := time.Now()
	stream, err := client.StreamPodLogsWithTimestamps(ctx, ns, pod, tail, container)
	if err != nil {
		sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, text: err.Error()})
		return
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), scannerBufferSize)

	for scanner.Scan() {
		line := scanner.Text()
		ts, body, err := parseLogTimestamp(line)
		if err != nil {
			// Unparseable lines (should be rare with Timestamps=true) are treated
			// as live and keep the raw text so nothing is lost.
			ts, body = time.Time{}, line
		}
		live := ts.IsZero() || !ts.Before(openTime.Add(-historyTimeTolerance))
		if !sendParsed(ctx, out, parsedEvent{pod: pod, text: body, ts: ts, kind: lineLog, live: live}) {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, text: err.Error()})
		return
	}
	sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamDone})
}

func sendParsed(ctx context.Context, ch chan<- parsedEvent, ev parsedEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}

// parseLogTimestamp strips the RFC3339Nano timestamp that Kubernetes prepends
// when Timestamps=true. It returns the timestamp and the remainder of the line.
func parseLogTimestamp(line string) (time.Time, string, error) {
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return time.Time{}, "", errors.New("no timestamp prefix")
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:i])
	if err != nil {
		return time.Time{}, "", err
	}
	return ts, line[i+1:], nil
}

// compile-time assertion: *k8s.Client satisfies the streamer interface.
var _ streamer = (*k8s.Client)(nil)
