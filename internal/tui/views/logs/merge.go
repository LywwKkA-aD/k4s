package logs

import (
	"context"
	"io"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// mergeIncomingBufferCap is how many events per pod the pump's inbox
// holds before a reader waits.
const mergeIncomingBufferCap = 16

// streamer is the subset of *k8s.Client the streams need. It keeps tests
// free of a real Kubernetes clientset.
type streamer interface {
	StreamLogs(ctx context.Context, namespace, pod string, opts k8s.LogOptions) (io.ReadCloser, error)
}

// passLoop hands one pod's events on as they come, until ctx is done.
func passLoop(ctx context.Context, incoming <-chan parsedEvent, events chan<- logEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-incoming:
			if !ok || !emitParsed(ctx, events, []parsedEvent{ev}) {
				return
			}
		}
	}
}

// mergeLoop feeds the merger the events as they arrive and the times it
// asks for, and sends on what it lets go, until ctx is done or incoming
// closes. now is the laptop's clock.
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
		out := logEvent{Pod: ev.pod, Attempt: ev.attempt}
		switch ev.kind {
		case lineLog:
			out.Line, out.TS = ev.text, ev.ts
		case lineStreamErr:
			out.Err, out.Done = ev.err, true
		default:
			out.Done = true
		}
		if !send(ctx, events, out) {
			return false
		}
	}
	return true
}

// compile-time assertion: *k8s.Client satisfies the streamer interface.
var _ streamer = (*k8s.Client)(nil)
