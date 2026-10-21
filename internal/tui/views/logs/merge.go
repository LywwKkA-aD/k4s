package logs

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// mergeIncomingBufferCap is how many events per pod the merge loop's
// inbox holds before a reader waits.
const mergeIncomingBufferCap = 16

// streamer is the subset of *k8s.Client the streams need. It keeps tests
// free of a real Kubernetes clientset.
type streamer interface {
	StreamLogs(ctx context.Context, namespace, pod string, opts k8s.LogOptions) (io.ReadCloser, error)
	ContainersForPod(ctx context.Context, namespace, name string) ([]string, error)
}

// runMergedStreams reads every pod's log with timestamps, one goroutine a
// pod, and merges them into events (see merger). It closes events when
// every reader has finished or ctx is cancelled.
func runMergedStreams(ctx context.Context, src streamer, namespace string, pods []string, tail int64, container string, events chan<- logEvent) {
	defer close(events)
	if len(pods) == 0 {
		return
	}
	incoming := make(chan parsedEvent, len(pods)*mergeIncomingBufferCap)
	var readers sync.WaitGroup
	for _, pod := range pods {
		readers.Add(1)
		go func() {
			defer readers.Done()
			streamOnePodMerged(ctx, src, namespace, pod, tail, container, incoming)
		}()
	}
	go func() {
		readers.Wait()
		close(incoming)
	}()
	mergeLoop(ctx, incoming, events, newMerger(pods, tail, time.Now()), time.Now)
}

// mergeLoop feeds the merger the events as they arrive and the times it
// asks for, and sends on what it lets go. now is the laptop's clock.
func mergeLoop(ctx context.Context, incoming <-chan parsedEvent, events chan<- logEvent, g *merger, now func() time.Time) {
	timer := time.NewTimer(historyCap)
	defer timer.Stop()
	for {
		var due <-chan time.Time
		if at, ok := g.deadline(); ok {
			timer.Reset(max(at.Sub(now()), 0))
			due = timer.C
		}
		var out []parsedEvent
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-incoming:
			if !ok {
				emitParsed(ctx, events, g.drain())
				return
			}
			out = g.add(ev, now())
		case <-due:
			out = g.tick(now())
		}
		if !emitParsed(ctx, events, out) {
			return
		}
	}
}

// emitParsed sends evs on as the view's events; false once ctx is done.
func emitParsed(ctx context.Context, events chan<- logEvent, evs []parsedEvent) bool {
	for _, ev := range evs {
		var out logEvent
		switch ev.kind {
		case lineLog:
			out = logEvent{Pod: ev.pod, Line: ev.text}
		case lineStreamErr:
			out = logEvent{Pod: ev.pod, Err: errors.New(ev.text)}
		case lineStreamDone:
			out = logEvent{Pod: ev.pod, Done: true}
		}
		if !send(ctx, events, out) {
			return false
		}
	}
	return true
}

func streamOnePodMerged(ctx context.Context, src streamer, ns, pod string, tail int64, container string, out chan<- parsedEvent) {
	container = resolveStreamContainer(ctx, src, ns, pod, container)
	stream, err := src.StreamLogs(ctx, ns, pod, k8s.LogOptions{Container: container, Tail: tail, Timestamps: true})
	if err != nil {
		sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, text: err.Error()})
		return
	}
	defer func() { _ = stream.Close() }()

	lines := newLineReader(stream)
	for {
		line, err := lines.next()
		if line.ok {
			ts, body := line.withoutTimestamp()
			if !sendParsed(ctx, out, parsedEvent{pod: pod, text: body.text(), ts: ts, kind: lineLog}) {
				return
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() == nil {
				sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, text: err.Error()})
			}
			return
		}
	}
	sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamDone})
}

// resolveStreamContainer guards the multi-pod flows (service / deployment
// wide): the container is picked from the first pod's spec, but a service
// selector can match pods from other workloads that don't have it. Instead
// of dying with the API's "container X is not valid for pod Y", fall back to
// the pod's own first container. On any lookup problem the preferred name is
// kept and the stream surfaces the real error as before.
func resolveStreamContainer(ctx context.Context, src streamer, ns, pod, container string) string {
	if container == "" {
		return ""
	}
	names, err := src.ContainersForPod(ctx, ns, pod)
	if err != nil || len(names) == 0 {
		return container
	}
	for _, n := range names {
		if n == container {
			return container
		}
	}
	return names[0]
}

func sendParsed(ctx context.Context, ch chan<- parsedEvent, ev parsedEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}

// compile-time assertion: *k8s.Client satisfies the streamer interface.
var _ streamer = (*k8s.Client)(nil)
