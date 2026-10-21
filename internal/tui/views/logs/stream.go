package logs

import (
	"context"
	"errors"
	"io"
)

// readStream opens r's log, always with timestamps: each line's stamp is
// the pod's resume point, and the view never shows it.
func readStream(ctx context.Context, src streamer, ns string, r attachReq, out chan<- parsedEvent) {
	opts := r.opts
	opts.Timestamps = true
	stream, err := src.StreamLogs(ctx, ns, r.pod, opts)
	if err != nil {
		if ctx.Err() == nil {
			sendParsed(ctx, out, parsedEvent{pod: r.pod, attempt: r.attempt, kind: lineStreamErr, err: err})
		}
		return
	}
	readOpen(ctx, stream, r, out)
}

// readOpen reads an open log stream into out and closes it: each line
// split from its stamp before it is made inert, the lines stamped at or
// before r.after dropped (a reconnect asked from the second they fall
// in), then the end of the stream. A line without a stamp is kept. A
// stream cut mid-line drops the part it got: the reconnect reads the
// whole line again, and its stamp must not count as shown.
func readOpen(ctx context.Context, stream io.ReadCloser, r attachReq, out chan<- parsedEvent) {
	defer func() { _ = stream.Close() }()

	lines := newLineReader(stream)
	for {
		line, err := lines.next()
		if line.ok && (err == nil || errors.Is(err, io.EOF)) {
			ts, body := line.withoutTimestamp()
			if ts.IsZero() || ts.After(r.after) {
				ev := parsedEvent{pod: r.pod, attempt: r.attempt, text: body.text(), ts: ts, kind: lineLog}
				if !sendParsed(ctx, out, ev) {
					return
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() == nil {
				sendParsed(ctx, out, parsedEvent{pod: r.pod, attempt: r.attempt, kind: lineStreamErr, err: err})
			}
			return
		}
	}
	sendParsed(ctx, out, parsedEvent{pod: r.pod, attempt: r.attempt, kind: lineStreamDone})
}

func send(ctx context.Context, ch chan<- logEvent, ev logEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}

func sendParsed(ctx context.Context, ch chan<- parsedEvent, ev parsedEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}
