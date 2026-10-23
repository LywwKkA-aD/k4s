package dash

import (
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// parseReadyz reads /readyz?verbose: "[+]ping ok", "[-]etcd failed: …", and
// a last line "readyz check passed" or "readyz check failed".
func parseReadyz(body []byte) readyzInfo {
	info := readyzInfo{known: true, etcd: StateUnknown}
	passedLine := false
	for line := range strings.Lines(string(body)) {
		line = strings.TrimSpace(line)
		ok, bad := strings.HasPrefix(line, "[+]"), strings.HasPrefix(line, "[-]")
		if !ok && !bad {
			passedLine = passedLine || strings.HasSuffix(line, "check passed")
			continue
		}
		name, _, _ := strings.Cut(line[3:], " ")
		info.checks++
		state := StateOK
		if ok {
			info.passed++
		} else {
			info.failed = append(info.failed, name)
			state = StateFailed
		}
		if name == "etcd" {
			info.etcd = state
		}
	}
	failedLine := strings.Contains(string(body), "check failed")
	info.ready = passedLine || (info.checks > 0 && len(info.failed) == 0 && !failedLine)
	return info
}

// minorOf reads the minor version, tolerating "31+" from managed servers.
func minorOf(minor string) int {
	n, err := strconv.Atoi(strings.TrimRight(minor, "+"))
	if err != nil {
		return 0
	}
	return n
}

// distroOf is a local stand-in for M4.2 detection: the git version suffix.
func distroOf(git string) string {
	switch {
	case strings.Contains(git, "+k3s"):
		return "k3s"
	case strings.Contains(git, "+rke2"):
		return "rke2"
	case strings.Contains(git, "-eks-"):
		return "eks"
	case strings.Contains(git, "-gke."):
		return "gke"
	default:
		return ""
	}
}

// distro is the distribution the server version names or, while it is
// unknown (denied, or not answered yet), the one every kubelet names: a
// k3s node reports v1.36.5+k3s1 too. The nodes list is fetched anyway, so
// this costs no request. Kubelets that disagree name none.
func (st store) distro() string {
	if st.version.git != "" {
		return distroOf(st.version.git)
	}
	var d string
	for i, n := range st.nodes {
		nd := distroOf(n.kubelet)
		if i > 0 && nd != d {
			return ""
		}
		d = nd
	}
	return d
}

// lbFieldSelectorMinor is the first release whose Service registry accepts
// spec.type in a field selector.
const lbFieldSelectorMinor = 31

func nsRowsFrom(t table, now time.Time) []nsRow {
	name, status, age := t.col("Name"), t.col("Status"), t.col("Age")
	out := make([]nsRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		r := nsRow{name: n}
		r.status, _ = t.str(i, status)
		if s, ok := t.str(i, age); ok {
			if d, ok := parseAge(s); ok {
				r.created = now.Add(-d)
			}
		}
		out = append(out, r)
	}
	return out
}

func statusesFrom(t table) []string {
	status := t.col("Status")
	out := make([]string, 0, t.len())
	for i := range t.len() {
		s, _ := t.str(i, status)
		out = append(out, s)
	}
	return out
}

// serviceRowsFrom reads the services Table: TYPE and EXTERNAL-IP, where a
// LoadBalancer without an address shows <pending>.
func serviceRowsFrom(t table, ns string) []svcRow {
	name, typ, ext := t.col("Name"), t.col("Type"), t.col("External-IP")
	out := make([]svcRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		r := svcRow{name: n, namespace: ns}
		r.typ, _ = t.str(i, typ)
		e, _ := t.str(i, ext)
		r.pending = r.typ == string(corev1.ServiceTypeLoadBalancer) && e == "<pending>"
		out = append(out, r)
	}
	return out
}

// loadBalancersFrom re-applies spec.type=LoadBalancer, which a fake
// clientset ignores; a balancer without ingress is pending.
func loadBalancersFrom(items []corev1.Service) []svcRow {
	out := make([]svcRow, 0, len(items))
	for _, s := range items {
		if s.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		out = append(out, svcRow{
			name: s.Name, namespace: s.Namespace, typ: string(s.Spec.Type),
			pending: len(s.Status.LoadBalancer.Ingress) == 0,
		})
	}
	return out
}

func deriveCluster(st store, led Ledger, scope Scope, host string, now time.Time) ClusterPanel {
	readyz := kubectl("get", "--raw=/readyz?verbose")
	p := ClusterPanel{
		Cmd:     readyz,
		API:     apiRow(st.readyz, availOf(Readyz, led), readyz),
		Version: versionRow(st, availOf(Version, led)),
		You:     TextRow{Avail: availOf(Whoami, led), Text: st.user, Cmd: kubectl("auth", "whoami")},
		Server:  TextRow{Avail: Avail{State: StateOK}, Text: host, Cmd: kubectl("cluster-info")},
		Age:     AgeRow{Avail: availOf(Namespaces, led), Cmd: kubectl("get", "ns", "kube-system")},
		Namespaces: NamespacesRow{
			Avail: availOf(Namespaces, led), Count: len(st.namespaces), Cmd: kubectl("get", "ns"),
		},
		Storage: storageRow(st.pvcs, availOf(PVCs, led), withScope(scope.Namespace, "get", "pvc")),
		Network: networkRow(st, led, scope),
	}
	for _, n := range st.namespaces {
		if n.status == "Terminating" {
			p.Namespaces.Terminating++
		}
		if n.name == "kube-system" && !n.created.IsZero() {
			p.Age.Age, p.Age.Known = since(now, n.created), true
		}
	}
	return p
}

func apiRow(r readyzInfo, a Avail, cmd Cmd) APIRow {
	row := APIRow{Avail: a, Ready: r.ready, Checks: r.checks, Passed: r.passed, Failed: r.failed, Etcd: r.etcd, Cmd: cmd}
	if r.known && !r.ready {
		row.Sev = SevFailing
	}
	return row
}

func versionRow(st store, a Avail) VersionRow {
	row := VersionRow{Avail: a, GitVersion: st.version.git, Distro: st.distro(), Cmd: kubectl("version")}
	for _, n := range st.nodes {
		row.EmbeddedEtcd = row.EmbeddedEtcd || n.etcd
		if n.controlPlane {
			row.ControlPlanes++
		}
	}
	return row
}

func storageRow(statuses []string, a Avail, cmd Cmd) StorageRow {
	row := StorageRow{Avail: a, Total: len(statuses), Cmd: cmd}
	for _, s := range statuses {
		switch s {
		case "Bound":
			row.Bound++
		case "Pending":
			row.Pending++
		case "Lost":
			row.Lost++
		}
	}
	if row.Pending > 0 || row.Lost > 0 {
		row.Sev = SevWaiting
	}
	return row
}

func networkRow(st store, led Ledger, scope Scope) NetworkRow {
	lbs := st.lbs
	row := NetworkRow{
		Avail: availOf(LoadBalancers, led),
		Cmd:   withScope(scope.Namespace, "get", "svc", "--field-selector", "spec.type=LoadBalancer"),
	}
	switch {
	case st.lbFallback:
		lbs = nil
		for _, s := range st.services {
			if s.typ == string(corev1.ServiceTypeLoadBalancer) {
				lbs = append(lbs, s)
			}
		}
		row.Services, row.ServicesKnown = len(st.services), led[LoadBalancers].State == StateOK || len(st.services) > 0
		row.Cmd = approx(withScope(scope.Namespace, "get", "svc"))
	case st.svcCountOK:
		row.Services, row.ServicesKnown, row.ServicesApprox = st.svcCount, true, st.svcCountMore
	}
	row.LoadBalancers = len(lbs)
	for _, s := range lbs {
		if s.pending {
			row.Pending++
		}
	}
	return row
}
