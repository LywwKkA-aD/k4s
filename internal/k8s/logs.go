package k8s

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// LogOptions says which log of a pod StreamLogs opens.
type LogOptions struct {
	// Container picks one container of a multi-container pod; "" lets the
	// API server choose the default container, as kubectl does.
	Container string
	// Tail seeds the stream with the last Tail lines; zero or less sends
	// the whole log (rarely what you want).
	Tail int64
	// Previous opens the log of the container's previous instance, the one
	// that ran before its last restart (kubectl logs --previous). That log
	// has ended, so it is read to the end rather than followed.
	Previous bool
	// Timestamps prefixes every line with its RFC3339Nano time, which the
	// caller strips before showing the line.
	Timestamps bool
}

// StreamLogs opens the named pod's log as o says, followed unless it is
// the previous instance's. The caller closes the stream, typically by
// deferring Close and cancelling ctx to interrupt a read. Errors are the
// API server's own, so a caller can classify them.
func (c *Client) StreamLogs(ctx context.Context, namespace, podName string, o LogOptions) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{
		Follow:     !o.Previous,
		Previous:   o.Previous,
		Timestamps: o.Timestamps,
		Container:  o.Container,
	}
	if o.Tail > 0 {
		tail := o.Tail
		opts.TailLines = &tail
	}
	return c.Clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
}

// StreamPodLogs returns a follow stream for the named pod's logs, seeded
// with the last tailLines lines (all of them for zero or less). container
// picks one container of a multi-container pod; "" lets Kubernetes choose
// the default container, as `kubectl logs` does.
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName string, tailLines int64, container string) (io.ReadCloser, error) {
	return c.StreamLogs(ctx, namespace, podName, LogOptions{Container: container, Tail: tailLines})
}

// StreamPodLogsWithTimestamps is StreamPodLogs with every line prefixed by
// an RFC3339Nano timestamp, which the caller strips before showing it.
func (c *Client) StreamPodLogsWithTimestamps(ctx context.Context, namespace, podName string, tailLines int64, container string) (io.ReadCloser, error) {
	return c.StreamLogs(ctx, namespace, podName, LogOptions{Container: container, Tail: tailLines, Timestamps: true})
}
