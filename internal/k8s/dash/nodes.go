package dash

import (
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	roleLabelPrefix = "node-role.kubernetes.io/"
	etcdRoleLabel   = roleLabelPrefix + "etcd"
	// reconcileAnnotation is the one node annotation the dashboard reads. k3s
	// writes it when it reconciles the snapshot list, not per snapshot.
	reconcileAnnotation = "etcd.k3s.cattle.io/local-snapshots-timestamp"
)

var pressureConditions = []corev1.NodeConditionType{
	corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable,
}

// nodesFrom keeps what the panel reads. Annotations are allow-listed here,
// so k3s.io/node-args and node-env never reach a snapshot.
func nodesFrom(items []corev1.Node) []nodeInfo {
	out := make([]nodeInfo, 0, len(items))
	for _, n := range items {
		info := nodeInfo{
			name:          n.Name,
			roles:         rolesOf(n.Labels),
			ready:         string(corev1.ConditionUnknown),
			unschedulable: n.Spec.Unschedulable,
			cpuMilli:      n.Status.Allocatable.Cpu().MilliValue(),
			memBytes:      n.Status.Allocatable.Memory().Value(),
			podsCap:       n.Status.Allocatable.Pods().Value(),
			created:       n.CreationTimestamp.Time,
			kubelet:       n.Status.NodeInfo.KubeletVersion,
			reconciledAt:  n.Annotations[reconcileAnnotation],
		}
		_, info.etcd = n.Labels[etcdRoleLabel]
		info.controlPlane = slices.Contains(info.roles, "control-plane") || slices.Contains(info.roles, "master")
		for _, c := range n.Status.Conditions {
			switch {
			case c.Type == corev1.NodeReady:
				info.ready = string(c.Status)
			case slices.Contains(pressureConditions, c.Type) && c.Status == corev1.ConditionTrue:
				info.pressures = append(info.pressures, string(c.Type))
			}
		}
		out = append(out, info)
	}
	return out
}

func rolesOf(labels map[string]string) []string {
	var roles []string
	for k := range labels {
		if role, ok := strings.CutPrefix(k, roleLabelPrefix); ok && role != "" {
			roles = append(roles, role)
		}
	}
	if r := labels["kubernetes.io/role"]; r != "" && !slices.Contains(roles, r) {
		roles = append(roles, r)
	}
	slices.Sort(roles)
	return roles
}

// quantityMilli and quantityValue read metrics.k8s.io quantities; a value
// that does not parse counts as zero, as in kubectl top.
func quantityMilli(s string) int64 {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.MilliValue()
}

func quantityValue(s string) int64 {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.Value()
}

func meter(used, capacity int64, known bool) Meter {
	if !known || capacity <= 0 {
		return Meter{}
	}
	pct := int(used * 100 / capacity)
	return Meter{Known: true, Used: used, Capacity: capacity, Percent: pct, Sev: meterSeverity(pct)}
}

func deriveNodes(st store, led Ledger, now time.Time) NodesPanel {
	p := NodesPanel{
		Avail:   availOf(Nodes, led),
		Metrics: availOf(NodeMetrics, led),
		Cmd:     kubectl("get", "nodes", "-o", "wide"),
		TopCmd:  approx(kubectl("top", "nodes")),
		Total:   len(st.nodes),
	}
	if !st.metricsOK && led[MetricsDiscovery].Fetched {
		p.Metrics.State = StateAbsent
	}
	usage := map[string]usageRow{}
	for _, m := range st.nodeMetrics {
		usage[m.name] = m
	}
	podsOn := map[string]int{}
	for _, r := range st.pods {
		podsOn[r.node]++
	}
	podsKnown := led[Pods].State == StateOK || len(st.pods) > 0
	var cpuUsed, cpuCap, memUsed, memCap int64
	for _, n := range st.nodes {
		m, hasMetrics := usage[n.name]
		row := NodeRow{
			Key:           ObjectKey{Kind: "node", Name: n.name},
			Roles:         n.roles,
			Condition:     conditionWord(n.ready),
			Pressures:     n.pressures,
			Unschedulable: n.unschedulable,
			CPU:           meter(m.cpuMilli, n.cpuMilli, hasMetrics),
			Memory:        meter(m.memBytes, n.memBytes, hasMetrics),
			Pods:          podsOn[n.name],
			PodsCap:       int(n.podsCap),
			PodsKnown:     podsKnown && !st.podsMissingNode(),
			Age:           since(now, n.created),
			Kubelet:       n.kubelet,
			Etcd:          n.etcd,
			ControlPlane:  n.controlPlane,
			Cmd:           describe("node", n.name, ""),
		}
		row.Sev = nodeSeverity(row)
		if row.Condition == "Ready" {
			p.Ready++
		}
		if hasMetrics {
			cpuUsed, cpuCap = cpuUsed+m.cpuMilli, cpuCap+n.cpuMilli
			memUsed, memCap = memUsed+m.memBytes, memCap+n.memBytes
		}
		p.Rows = append(p.Rows, row)
	}
	p.CPU = meter(cpuUsed, cpuCap, cpuCap > 0)
	p.Memory = meter(memUsed, memCap, memCap > 0)
	return p
}

func conditionWord(ready string) string {
	switch ready {
	case string(corev1.ConditionTrue):
		return "Ready"
	case string(corev1.ConditionFalse):
		return "NotReady"
	default:
		return "Unknown"
	}
}

// nodeSeverity: NotReady or Unknown fails, pressure or a hot meter is
// degraded, cordoned is worth knowing.
func nodeSeverity(r NodeRow) Severity {
	switch {
	case r.Condition != "Ready":
		return SevFailing
	case len(r.Pressures) > 0:
		return SevWaiting
	case r.CPU.Sev == SevWaiting || r.Memory.Sev == SevWaiting:
		return SevWaiting
	case r.Unschedulable:
		return SevInfo
	default:
		return SevOK
	}
}

func (s store) podsMissingNode() bool { return slices.Contains(s.podsMissing, "Node") }
