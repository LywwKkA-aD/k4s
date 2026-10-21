package logs

import (
	"context"
	"errors"
	"io"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// streamOnePod follows one pod's log into ch: every line made inert on the
// way in, then a closing event.
func streamOnePod(ctx context.Context, src streamer, ns, pod string, opts k8s.LogOptions, ch chan<- logEvent) {
	stream, err := src.StreamLogs(ctx, ns, pod, opts)
	if err != nil {
		send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
		return
	}
	defer func() { _ = stream.Close() }()

	lines := newLineReader(stream)
	for {
		line, err := lines.next()
		if line.ok && !send(ctx, ch, logEvent{Pod: pod, Line: line.text()}) {
			return
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() == nil {
				send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
			}
			return
		}
	}
	send(ctx, ch, logEvent{Pod: pod, Done: true})
}

func send(ctx context.Context, ch chan<- logEvent, ev logEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}
