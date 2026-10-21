package dash

import (
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	kubeSystem   = "kube-system"
	svclbPrefix  = "svclb-"
	helmJobStart = "helm-install-"
)

// k3sComponents are the bundled components matched by name across the
// deployment and daemonset rows. On stock k3s traefik is a Deployment and
// ServiceLB runs one svclb-<service> DaemonSet per LoadBalancer.
var k3sComponents = []string{"coredns", "traefik", "metrics-server", "local-path-provisioner"}

func isComponent(name string) bool {
	return slices.Contains(k3sComponents, name) || strings.HasPrefix(name, svclbPrefix)
}

func helmRowsFrom(t table) []helmRow {
	name, job := t.col("Name"), t.col("Job")
	out := make([]helmRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		j, ok := t.str(i, job)
		if !ok || j == "" {
			j = helmJobStart + n
		}
		out = append(out, helmRow{name: n, job: j})
	}
	return out
}

// snapshotRowsFrom reads the etcdsnapshotfiles Table. CreationTime is a
// date column, which the server renders as an age; Age is the fallback.
func snapshotRowsFrom(t table, now time.Time) []snapRow {
	node, created := t.col("Node"), t.col("CreationTime")
	if created < 0 {
		created = t.col("Age")
	}
	out := make([]snapRow, 0, t.len())
	for i := range t.len() {
		s, ok := t.str(i, created)
		if !ok {
			continue
		}
		d, ok := parseAge(s)
		if !ok {
			continue
		}
		n, _ := t.str(i, node)
		out = append(out, snapRow{node: n, created: now.Add(-d)})
	}
	return out
}

// placedComponent is a deploy or ds row in kube-system. placed is false
// while its lookup is still queued; it then counts as present but its
// command finds it by name.
type placedComponent struct {
	kind   string
	row    workloadRow
	placed bool
}

// component builds one component row. A missing component is "off": dim,
// never red, since k3s lets each one be disabled.
func component(name string, byName map[string]placedComponent) Component {
	c, ok := byName[name]
	if !ok {
		return Component{Key: ObjectKey{Kind: "deployment", Namespace: kubeSystem, Name: name},
			Cmd: describe("deployment", name, kubeSystem)}
	}
	comp := Component{
		Key:     ObjectKey{Kind: c.kind, Namespace: kubeSystem, Name: name},
		Present: true, Ready: c.row.ready, Desired: c.row.desired,
		Sev: workloadSeverity(c.row.ready, c.row.desired),
		Cmd: describe(c.kind, name, kubeSystem),
	}
	if !c.placed {
		comp.Key.Namespace = ""
		comp.Cmd = workloadCmd(comp.Key, false)
	}
	return comp
}

func deriveK3s(st store, led Ledger, scope Scope, distro string, now time.Time) K3sPanel {
	p := K3sPanel{Cmd: kubectl("get", "deploy,ds", "-n", kubeSystem)}
	if distro != "k3s" {
		p.Hidden = "not k3s"
		return p
	}
	inScope := scope.Namespace == "" || scope.Namespace == kubeSystem
	if !inScope || led[Deployments].State != StateOK {
		p.Hidden = "no kube-system access"
		return p
	}
	p.Shown = true
	found := kubeSystemComponents(st, scope)
	byName := map[string]placedComponent{}
	for _, c := range found {
		byName[c.row.name] = c
	}
	for _, name := range k3sComponents {
		p.Components = append(p.Components, component(name, byName))
	}
	p.ServiceLB = serviceLB(found, st, scope)
	p.Helm = helmSummary(st, led, scope)
	p.Etcd = etcdRow(st, led, now)
	return p
}

// kubeSystemComponents returns component rows that are in kube-system:
// from a kube-system list directly, else from the cached lookups.
func kubeSystemComponents(st store, scope Scope) []placedComponent {
	var out []placedComponent
	for _, kr := range []kindRows{{"deployment", st.deploys}, {"daemonset", st.ds}} {
		used := map[string][]bool{}
		for _, r := range kr.rows {
			if !isComponent(r.name) {
				continue
			}
			if scope.Namespace == kubeSystem {
				out = append(out, placedComponent{kr.kind, r, true})
				continue
			}
			key, placed := placeWorkload(st, kr.kind, r, used)
			_, looked := st.resolved[resolveKey{kr.kind, r.name}]
			switch {
			case placed && key.Namespace == kubeSystem:
				out = append(out, placedComponent{kr.kind, r, true})
			case !looked:
				out = append(out, placedComponent{kr.kind, r, false})
			}
		}
	}
	return out
}

func serviceLB(found []placedComponent, st store, scope Scope) ServiceLBRow {
	row := ServiceLBRow{Cmd: withScope(scope.Namespace, "get", "svc", "--field-selector", "spec.type=LoadBalancer")}
	for _, c := range found {
		if c.kind != "daemonset" || !strings.HasPrefix(c.row.name, svclbPrefix) {
			continue
		}
		row.Total++
		if workloadSeverity(c.row.ready, c.row.desired) == SevOK {
			row.Ready++
		}
	}
	lbs := st.lbs
	if st.lbFallback {
		lbs = st.services
		row.Cmd = approx(withScope(scope.Namespace, "get", "svc"))
	}
	for _, s := range lbs {
		if s.pending {
			row.Pending++
		}
	}
	switch {
	case row.Total > 0:
		row.State = StateOK
	case row.Pending > 0:
		row.State = StateAbsent // pending balancers and no svclb: ServiceLB is off
	default:
		row.State = StateUnknown // no LoadBalancer services, nothing to tell
	}
	return row
}

func helmSummary(st store, led Ledger, scope Scope) HelmRow {
	row := HelmRow{Avail: availOf(HelmCharts, led), Cmd: approx(withScope(scope.Namespace, "get", "helmcharts"))}
	failedJobs := map[string]bool{}
	for _, j := range st.jobs {
		if j.failed {
			failedJobs[j.name] = true
		}
	}
	for _, h := range st.helm {
		row.Charts = append(row.Charts, h.name)
		if failedJobs[h.job] {
			row.Failed++
		}
	}
	sort.Strings(row.Charts)
	return row
}

// etcdRow prefers the newest ETCDSnapshotFile; without that API it falls
// back to the reconcile annotation and says so. kine/SQLite servers carry
// no etcd role label and get no row.
func etcdRow(st store, led Ledger, now time.Time) EtcdRow {
	var etcdNode string
	for _, n := range st.nodes {
		if n.etcd && etcdNode == "" {
			etcdNode = n.name
		}
	}
	if etcdNode == "" {
		return EtcdRow{}
	}
	row := EtcdRow{Shown: true, Cmd: kubectl("get", "etcdsnapshotfiles")}
	if led[EtcdSnapshots].State != StateAbsent {
		var newest snapRow
		for _, s := range st.snapshots {
			if s.created.After(newest.created) {
				newest = s
			}
		}
		if !newest.created.IsZero() {
			row.Known, row.Ago, row.Node = true, since(now, newest.created), newest.node
		}
		return row
	}
	row.Reconciled, row.Node = true, etcdNode
	row.Cmd = kubectl("get", "node", etcdNode, "-o",
		`jsonpath={.metadata.annotations.etcd\.k3s\.cattle\.io/local-snapshots-timestamp}`)
	for _, n := range st.nodes {
		if n.name != etcdNode {
			continue
		}
		if t, err := time.Parse(time.RFC3339, n.reconciledAt); err == nil {
			row.Known, row.Ago = true, since(now, t)
		}
	}
	return row
}
