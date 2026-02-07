package k8s

import (
	"bufio"
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/logger"
)

// LogOptions configures log retrieval
type LogOptions struct {
	Container  string
	Follow     bool
	TailLines  int64
	Timestamps bool
	Previous   bool
}

// DefaultLogOptions returns sensible default log options
func DefaultLogOptions() LogOptions {
	return LogOptions{
		TailLines:  500,
		Timestamps: false,
		Follow:     false,
		Previous:   false,
	}
}

// GetPodLogs retrieves logs for a pod container
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName string, opts LogOptions) (string, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	logger.Debug("GetPodLogs", "pod", podName, "container", opts.Container, "namespace", namespace, "tailLines", opts.TailLines)

	podLogOpts := &corev1.PodLogOptions{
		Container:  opts.Container,
		Follow:     false, // Non-streaming
		Timestamps: opts.Timestamps,
		Previous:   opts.Previous,
	}

	if opts.TailLines > 0 {
		podLogOpts.TailLines = &opts.TailLines
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		logger.Error("GetPodLogs: failed to get stream", "err", err)
		return "", fmt.Errorf("get log stream: %w", err)
	}
	defer stream.Close()

	logs, err := io.ReadAll(stream)
	if err != nil {
		logger.Error("GetPodLogs: failed to read logs", "err", err)
		return "", fmt.Errorf("read logs: %w", err)
	}

	logger.Debug("GetPodLogs: read bytes", "bytes", len(logs))
	return string(logs), nil
}

// StreamPodLogs streams logs for a pod container, sending each line to the provided channel
// The function returns when the context is cancelled or an error occurs
func (c *Client) StreamPodLogs(ctx context.Context, namespace, podName string, opts LogOptions, lineChan chan<- string) error {
	if namespace == "" {
		namespace = c.namespace
	}

	logger.Debug("StreamPodLogs: starting stream", "pod", podName, "container", opts.Container)

	podLogOpts := &corev1.PodLogOptions{
		Container:  opts.Container,
		Follow:     true, // Streaming mode
		Timestamps: opts.Timestamps,
		Previous:   opts.Previous,
	}

	// For follow mode, use SinceSeconds to only get new logs (not historical)
	// TailLines=0 means don't fetch any historical logs, just stream new ones
	if opts.TailLines > 0 {
		podLogOpts.TailLines = &opts.TailLines
	} else {
		// When TailLines is 0, we want to stream only NEW logs
		// Use SinceSeconds=1 to start from ~now
		sinceSeconds := int64(1)
		podLogOpts.SinceSeconds = &sinceSeconds
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		logger.Error("StreamPodLogs: failed to get stream", "err", err)
		return fmt.Errorf("get log stream: %w", err)
	}
	defer stream.Close()

	reader := bufio.NewReader(stream)
	for {
		// Check context before blocking read
		select {
		case <-ctx.Done():
			logger.Debug("StreamPodLogs: context cancelled")
			return ctx.Err()
		default:
		}

		// ReadString will block until a newline or error
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// EOF in follow mode means pod terminated or connection closed
				// Send any remaining content
				if line != "" {
					select {
					case lineChan <- line:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				logger.Debug("StreamPodLogs: EOF reached (pod may have terminated)")
				return nil
			}
			// Check if context was cancelled (causes read error)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			logger.Error("StreamPodLogs: read error", "err", err)
			return fmt.Errorf("read log line: %w", err)
		}

		select {
		case lineChan <- line:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// GetPodContainers returns the list of container names for a pod
func (c *Client) GetPodContainers(ctx context.Context, namespace, podName string) ([]string, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s: %w", podName, err)
	}

	containers := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
	}

	return containers, nil
}
