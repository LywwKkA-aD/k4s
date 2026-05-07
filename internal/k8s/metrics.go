package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

// PodMetric is a presentation-friendly snapshot of one pod's resource use,
// summed across all of its containers. CPU is in millicores (1000m = 1 core)
// and memory is in bytes — both raw, formatted by the TUI on render.
type PodMetric struct {
	Name      string
	Namespace string
	CPUMilli  int64
	MemBytes  int64
}

// NodeMetric is the same idea for a single node.
type NodeMetric struct {
	Name     string
	CPUMilli int64
	MemBytes int64
}

// podMetricsItem mirrors the bit of metrics.k8s.io/v1beta1 PodMetrics we
// actually consume; we deliberately don't pull k8s.io/metrics in just for
// the typed clientset since the wire shape is stable and tiny.
type podMetricsItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Containers []struct {
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"containers"`
}

type podMetricsList struct {
	Items []podMetricsItem `json:"items"`
}

type nodeMetricsItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Usage struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	} `json:"usage"`
}

type nodeMetricsList struct {
	Items []nodeMetricsItem `json:"items"`
}

// ListPodMetrics queries the metrics-server for per-pod CPU/memory in the
// given namespace ("" = all). Returns a typed error when metrics-server is
// not installed so the TUI can render a friendly hint instead of a stack
// trace.
func (c *Client) ListPodMetrics(ctx context.Context, namespace string) ([]PodMetric, error) {
	path := "/apis/metrics.k8s.io/v1beta1/pods"
	if namespace != "" {
		path = "/apis/metrics.k8s.io/v1beta1/namespaces/" + namespace + "/pods"
	}
	data, err := c.Clientset.Discovery().RESTClient().Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics request: %w", err)
	}
	var list podMetricsList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode pod metrics: %w", err)
	}
	out := make([]PodMetric, 0, len(list.Items))
	for _, item := range list.Items {
		var cpuMilli, memBytes int64
		for _, ct := range item.Containers {
			cpuMilli += parseCPUMilli(ct.Usage.CPU)
			memBytes += parseMemoryBytes(ct.Usage.Memory)
		}
		out = append(out, PodMetric{
			Name:      item.Metadata.Name,
			Namespace: item.Metadata.Namespace,
			CPUMilli:  cpuMilli,
			MemBytes:  memBytes,
		})
	}
	return out, nil
}

// ListNodeMetrics queries the metrics-server for per-node CPU/memory.
func (c *Client) ListNodeMetrics(ctx context.Context) ([]NodeMetric, error) {
	const path = "/apis/metrics.k8s.io/v1beta1/nodes"
	data, err := c.Clientset.Discovery().RESTClient().Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics request: %w", err)
	}
	var list nodeMetricsList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode node metrics: %w", err)
	}
	out := make([]NodeMetric, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, NodeMetric{
			Name:     item.Metadata.Name,
			CPUMilli: parseCPUMilli(item.Usage.CPU),
			MemBytes: parseMemoryBytes(item.Usage.Memory),
		})
	}
	return out, nil
}

// parseCPUMilli converts a Kubernetes CPU quantity ("10m", "0.25", "2") to
// millicores. Bad input returns 0 — metrics are best-effort observability,
// not authoritative state, so we silently drop unparseable values rather
// than fail the whole list.
func parseCPUMilli(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.MilliValue()
}

// parseMemoryBytes converts a Kubernetes memory quantity ("100Ki", "2Gi")
// to a byte count. Bad input returns 0 (see parseCPUMilli for rationale).
func parseMemoryBytes(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.Value()
}

// FormatCPU renders millicores as kubectl-style "100m" / "1.250".
// Matches `kubectl top` rounding so users see familiar numbers.
func FormatCPU(milli int64) string {
	if milli < 1000 {
		return fmt.Sprintf("%dm", milli)
	}
	return fmt.Sprintf("%d.%03d", milli/1000, milli%1000)
}

// FormatMemory renders bytes as kubectl-style "12Mi" / "1.2Gi".
// We collapse to the largest unit that keeps the number readable; same
// behaviour as `kubectl top` so the TUI feels native.
func FormatMemory(bytes int64) string {
	const (
		Ki = 1024
		Mi = 1024 * Ki
		Gi = 1024 * Mi
	)
	switch {
	case bytes >= Gi:
		return fmt.Sprintf("%dGi", bytes/Gi)
	case bytes >= Mi:
		return fmt.Sprintf("%dMi", bytes/Mi)
	case bytes >= Ki:
		return fmt.Sprintf("%dKi", bytes/Ki)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
