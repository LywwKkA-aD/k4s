package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// ErrMetricsUnavailable is wrapped by a metrics query when the cluster
// serves no metrics API: metrics-server is not installed (404), not ready
// (503), or the client has no discovery REST client to ask. RBAC refusing
// metrics is not this error but a Forbidden.
var ErrMetricsUnavailable = errors.New("metrics API unavailable")

// metricsAPI is the metrics.k8s.io/v1beta1 path, as AbsPath segments.
var metricsAPI = []string{"/apis", "metrics.k8s.io", "v1beta1"}

// ListPodMetrics queries the metrics-server for per-pod CPU/memory in the
// given namespace ("" = all). A cluster without a serving metrics API gives
// ErrMetricsUnavailable; every failure is a *CallError for
// pods.metrics.k8s.io.
func (c *Client) ListPodMetrics(ctx context.Context, namespace string) ([]PodMetric, error) {
	segs := slices.Clone(metricsAPI)
	if namespace != "" {
		if err := ValidateNamespace(namespace); err != nil {
			return nil, listError("pods.metrics.k8s.io", namespace, err)
		}
		segs = append(segs, "namespaces", namespace)
	}
	data, err := c.metricsGet(ctx, append(segs, "pods"))
	if err != nil {
		return nil, listError("pods.metrics.k8s.io", namespace, err)
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

// ListNodeMetrics queries the metrics-server for per-node CPU/memory, with
// the errors of ListPodMetrics for nodes.metrics.k8s.io.
func (c *Client) ListNodeMetrics(ctx context.Context) ([]NodeMetric, error) {
	data, err := c.metricsGet(ctx, append(slices.Clone(metricsAPI), "nodes"))
	if err != nil {
		return nil, &CallError{Verb: "list", Resource: "nodes.metrics.k8s.io", ClusterScoped: true, Err: err}
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

// metricsGet GETs the metrics API path segs. Result.Error decodes a Status
// body, which DoRaw never does, so a 403 keeps the apiserver's wording of
// what was denied.
func (c *Client) metricsGet(ctx context.Context, segs []string) ([]byte, error) {
	rc := c.Clientset.Discovery().RESTClient()
	if rc == nil {
		return nil, ErrMetricsUnavailable
	}
	res := rc.Get().AbsPath(segs...).Do(ctx)
	data, err := res.Raw()
	if err != nil {
		err = res.Error()
	}
	if apierrors.IsNotFound(err) || apierrors.IsServiceUnavailable(err) {
		return nil, fmt.Errorf("%w: %w", ErrMetricsUnavailable, err)
	}
	return data, err
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
