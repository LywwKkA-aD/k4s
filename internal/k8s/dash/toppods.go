package dash

import (
	"encoding/json"
	"fmt"
	"sort"
)

const topPodRows = 8

type wireUsage struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type wireMetricsList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Usage      wireUsage `json:"usage"`
		Containers []struct {
			Usage wireUsage `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

// metricsFrom decodes a NodeMetricsList or PodMetricsList; pods sum their
// containers, as kubectl top pods does.
func metricsFrom(body []byte) ([]usageRow, error) {
	var w wireMetricsList
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("decode metrics: %w", err)
	}
	out := make([]usageRow, 0, len(w.Items))
	for _, it := range w.Items {
		r := usageRow{name: it.Metadata.Name, namespace: it.Metadata.Namespace}
		if len(it.Containers) == 0 {
			r.cpuMilli, r.memBytes = quantityMilli(it.Usage.CPU), quantityValue(it.Usage.Memory)
		}
		for _, c := range it.Containers {
			r.cpuMilli += quantityMilli(c.Usage.CPU)
			r.memBytes += quantityValue(c.Usage.Memory)
		}
		out = append(out, r)
	}
	return out, nil
}

func deriveTopPods(st store, led Ledger, scope Scope) TopPodsPanel {
	p := TopPodsPanel{
		Avail:     availOf(PodMetrics, led),
		CmdCPU:    withScope(scope.Namespace, "top", "pods", "--sort-by=cpu"),
		CmdMemory: withScope(scope.Namespace, "top", "pods", "--sort-by=memory"),
		Reporting: len(st.podMetrics),
	}
	if !st.metricsOK && led[MetricsDiscovery].Fetched {
		p.Avail.State = StateAbsent
	}
	p.ByCPU = topBy(st.podMetrics, func(r usageRow) int64 { return r.cpuMilli })
	p.ByMemory = topBy(st.podMetrics, func(r usageRow) int64 { return r.memBytes })
	return p
}

func topBy(rows []usageRow, by func(usageRow) int64) []PodUsage {
	sorted := append([]usageRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if by(sorted[i]) != by(sorted[j]) {
			return by(sorted[i]) > by(sorted[j])
		}
		return sorted[i].namespace+"/"+sorted[i].name < sorted[j].namespace+"/"+sorted[j].name
	})
	out := make([]PodUsage, 0, min(len(sorted), topPodRows))
	for _, r := range sorted[:min(len(sorted), topPodRows)] {
		out = append(out, PodUsage{
			Key:      ObjectKey{Kind: "pod", Namespace: r.namespace, Name: r.name},
			CPUMilli: r.cpuMilli, MemBytes: r.memBytes,
			Cmd: describe("pod", r.name, r.namespace),
		})
	}
	return out
}

// FormatCPU renders millicores the way kubectl top does: always "1420m".
func FormatCPU(milli int64) string { return fmt.Sprintf("%dm", milli) }

// FormatMemory renders bytes the way kubectl top does: always "3172Mi".
func FormatMemory(b int64) string { return fmt.Sprintf("%dMi", b>>20) }
