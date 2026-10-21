package dash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

// wireTableOut is the server-side Table the fake apiserver writes.
type wireTableOut struct {
	Kind              string           `json:"kind"`
	APIVersion        string           `json:"apiVersion"`
	Metadata          map[string]any   `json:"metadata"`
	ColumnDefinitions []map[string]any `json:"columnDefinitions"`
	Rows              []wireRowOut     `json:"rows"`
}

type wireRowOut struct {
	Cells  []any `json:"cells"`
	Object any   `json:"object"`
}

func columns(names ...string) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"name": n, "type": "string", "format": "", "description": "", "priority": 0})
	}
	return out
}

func age(d time.Duration) string { return duration.HumanDuration(d) }

func restartsCell(n int32, ago time.Duration) string {
	if n == 0 || ago == 0 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%d (%s ago)", n, age(ago))
}

// tableBuilder renders one resource's rows into t; in filters namespaces.
// It returns false when the cluster does not serve the API.
type tableBuilder func(c cluster, t *wireTableOut, in func(string) bool) bool

func addRow(t *wireTableOut, cells ...any) { t.Rows = append(t.Rows, wireRowOut{Cells: cells}) }

func workloadBuilder(kind string) tableBuilder {
	return func(c cluster, t *wireTableOut, in func(string) bool) bool { return c.workloadTable(t, kind, in) }
}

var tableBuilders = map[string]tableBuilder{
	"pods":                   podsTable,
	"deployments":            workloadBuilder("deployment"),
	"statefulsets":           workloadBuilder("statefulset"),
	"daemonsets":             workloadBuilder("daemonset"),
	"cronjobs":               cronTable,
	"persistentvolumeclaims": pvcTable,
	"namespaces":             nsTable,
	"services":               servicesTable,
	"helmcharts":             helmTable,
	"etcdsnapshotfiles":      snapshotsTable,
}

// table renders resource as a Table for namespace ns ("" for all).
func (c cluster) table(resource, ns string) (wireTableOut, bool) {
	t := wireTableOut{Kind: "Table", APIVersion: "meta.k8s.io/v1", Metadata: map[string]any{"resourceVersion": "4242"}}
	build, ok := tableBuilders[resource]
	if !ok {
		return t, false
	}
	return t, build(c, &t, func(objNS string) bool { return ns == "" || objNS == ns })
}

func podsTable(c cluster, t *wireTableOut, in func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Ready", "Status", "Restarts", "Age", "IP", "Node", "Nominated Node", "Readiness Gates")
	for i, p := range c.pods {
		if !in(p.ns) {
			continue
		}
		node := p.node
		if node == "" {
			node = "<none>"
		}
		addRow(t, p.name, fmt.Sprintf("%d/%d", p.ready, p.of), p.status, restartsCell(p.restarts, p.lastRestart),
			age(p.age), fmt.Sprintf("10.42.%d.%d", i/250, i%250), node, "<none>", "<none>")
	}
	return true
}

func cronTable(c cluster, t *wireTableOut, in func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Schedule", "Timezone", "Suspend", "Active", "Last Schedule", "Age", "Containers", "Images", "Selector")
	for _, cj := range c.crons {
		if in(cj.ns) {
			addRow(t, cj.name, "*/5 * * * *", "<none>", cj.suspend, 0, age(cj.last), "30d", "job", "busybox:1.36", "<none>")
		}
	}
	return true
}

func pvcTable(c cluster, t *wireTableOut, in func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Status", "Volume", "Capacity", "Access Modes", "StorageClass", "VolumeAttributesClass", "Age", "VolumeMode")
	for _, p := range c.pvcs {
		if in(p.ns) {
			addRow(t, p.name, p.status, "pvc-"+p.name, "1Gi", "RWO", "local-path", "<unset>", "5d", "Filesystem")
		}
	}
	return true
}

func nsTable(c cluster, t *wireTableOut, _ func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Status", "Age")
	for _, n := range c.namespaces {
		addRow(t, n.name, n.status, age(n.age))
	}
	return true
}

func servicesTable(c cluster, t *wireTableOut, in func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Type", "Cluster-IP", "External-IP", "Port(s)", "Age", "Selector")
	for _, s := range c.svcs {
		if !in(s.ns) {
			continue
		}
		ext := "<none>"
		switch {
		case s.typ == corev1.ServiceTypeLoadBalancer && s.pending:
			ext = "<pending>"
		case s.typ == corev1.ServiceTypeLoadBalancer:
			ext = "10.0.0.50"
		}
		addRow(t, s.name, string(s.typ), "10.43.0.10", ext, "80/TCP", "1d", "app="+s.name)
	}
	return true
}

func helmTable(c cluster, t *wireTableOut, _ func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "Job", "Chart", "TargetNamespace", "Version", "Repo", "HelmVersion", "Bootstrap")
	for _, h := range c.helm {
		addRow(t, h.name, h.job, "https://%{KUBERNETES_API}%/static/charts/"+h.name+".tgz", "kube-system", "", "", "", false)
	}
	return c.helm != nil
}

func snapshotsTable(c cluster, t *wireTableOut, _ func(string) bool) bool {
	t.ColumnDefinitions = columns("Name", "SnapshotName", "Node", "Location", "Size", "CreationTime")
	for i, s := range c.snaps {
		addRow(t, fmt.Sprintf("local-etcd-snapshot-%d", i), "etcd-snapshot", s.node, "file:///var/lib/rancher", "4501536", age(s.ago))
	}
	return c.snaps != nil
}

func (c cluster) workloadTable(t *wireTableOut, kind string, in func(string) bool) bool {
	switch kind {
	case "deployment":
		t.ColumnDefinitions = columns("Name", "Ready", "Up-to-date", "Available", "Age", "Containers", "Images", "Selector")
	case "statefulset":
		t.ColumnDefinitions = columns("Name", "Ready", "Age", "Containers", "Images")
	default:
		t.ColumnDefinitions = columns("Name", "Desired", "Current", "Ready", "Up-to-date", "Available", "Node Selector", "Age", "Containers", "Images", "Selector")
	}
	for _, w := range c.workloads {
		if w.kind != kind || !in(w.ns) {
			continue
		}
		ready := fmt.Sprintf("%d/%d", w.ready, w.desired)
		switch kind {
		case "deployment":
			addRow(t, w.name, ready, w.desired, w.ready, "10d", "app", "registry.example/app:1.0", "app="+w.name)
		case "statefulset":
			addRow(t, w.name, ready, "10d", "app", "registry.example/app:1.0")
		default:
			addRow(t, w.name, w.desired, w.desired, w.ready, w.desired, w.ready, "<none>", "10d", "app", "registry.example/app:1.0", "app="+w.name)
		}
	}
	return true
}

// metricsJSON renders a metrics.k8s.io list of nodes or pods.
func (c cluster) metricsJSON(resource, ns string) []byte {
	type usage struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	}
	type item struct {
		Metadata   map[string]string `json:"metadata"`
		Usage      *usage            `json:"usage,omitempty"`
		Containers []map[string]any  `json:"containers,omitempty"`
	}
	var items []item
	if resource == "nodes" {
		for _, n := range c.nodes {
			if n.useCPU != "" {
				items = append(items, item{Metadata: map[string]string{"name": n.name}, Usage: &usage{n.useCPU, n.useMem}})
			}
		}
	} else {
		for _, p := range c.podUse {
			if ns == "" || p.ns == ns {
				items = append(items, item{
					Metadata:   map[string]string{"name": p.name, "namespace": p.ns},
					Containers: []map[string]any{{"name": "app", "usage": usage{p.cpu, p.mem}}},
				})
			}
		}
	}
	body, _ := json.Marshal(map[string]any{"kind": "List", "apiVersion": "metrics.k8s.io/v1beta1", "items": items})
	return body
}

func writeJSONBody(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
