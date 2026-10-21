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
// every reader has finished or ctx is cancelled. A panic in a reader ends
// that pod's stream with an internal error; one in the merge ends the
// merge, and the readers with it.
func runMergedStreams(ctx context.Context, spec streamSpec, events chan<- logEvent) {
	defer close(events)
	if len(spec.pods) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	incoming := make(chan parsedEvent, len(spec.pods)*mergeIncomingBufferCap)
	var readers sync.WaitGroup
	for _, pod := range spec.pods {
		readers.Add(1)
		go func() {
			defer readers.Done()
			defer spec.crash.guard("the merged log stream of pod "+pod, func(err error) {
				sendParsed(ctx, incoming, parsedEvent{pod: pod, kind: lineStreamErr, err: err})
			})
			streamOnePodMerged(ctx, spec, pod, incoming)
		}()
	}
	go func() {
		readers.Wait()
		close(incoming)
	}()
	mergeGuarded(ctx, incoming, events, newMerger(spec.pods, spec.tail, time.Now()), time.Now, spec.crash)
}

// mergeGuarded is mergeLoop with a panic in it turned into an error event.
func mergeGuarded(ctx context.Context, incoming <-chan parsedEvent, events chan<- logEvent, g *merger, now func() time.Time, crash crashLog) {
	defer crash.guard("the log merge", func(err error) {
		send(ctx, events, logEvent{Err: err, Done: true})
	})
	mergeLoop(ctx, incoming, events, g, now)
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
			out = logEvent{Pod: ev.pod, Err: ev.err}
		case lineStreamDone:
			out = logEvent{Pod: ev.pod, Done: true}
		}
		if !send(ctx, events, out) {
			return false
		}
	}
	return true
}

// streamOnePodMerged reads one pod's log for the merge: each line split
// from its timestamp, made inert, then the end of the stream.
func streamOnePodMerged(ctx context.Context, spec streamSpec, pod string, out chan<- parsedEvent) {
	container := resolveStreamContainer(ctx, spec.src, spec.namespace, pod, spec.container)
	opts := k8s.LogOptions{Container: container, Tail: spec.tail, Timestamps: true}
	stream, err := spec.src.StreamLogs(ctx, spec.namespace, pod, opts)
	if err != nil {
		sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, err: err})
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
				sendParsed(ctx, out, parsedEvent{pod: pod, kind: lineStreamErr, err: err})
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
