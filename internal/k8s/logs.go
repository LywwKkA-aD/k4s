package k8s

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// StreamPodLogs returns a follow stream for the named pod's logs. The caller
// is responsible for closing the stream — typically by deferring Close() and
// cancelling the context to interrupt the read.
//
// tailLines seeds the stream with the last N lines before following:
//   - 0  → start from the very beginning (rarely what you want)
//   - >0 → kubectl logs --tail=N -f equivalent
//
// container picks one container in a multi-container pod; pass "" to let
// Kubernetes choose the default container (matches `kubectl logs` behaviour).
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName string, tailLines int64, container string) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{Follow: true}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	if container != "" {
		opts.Container = container
	}
	req := c.Clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	return req.Stream(ctx)
}
