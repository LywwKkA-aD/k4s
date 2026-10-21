package dash

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// render dumps a snapshot as stable text for golden files. The API host is
// left out: httptest picks a new port every run.
func render(s Snapshot) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	w("scope: %q fallback=%v", s.Scope.Namespace, s.Scope.Fallback)
	w("verdict: %s", s.Verdict)
	w("next:")
	for _, n := range s.Next {
		w("  %s", n)
	}
	if len(s.Next) == 0 {
		w("  daily: %s", s.Daily)
	}
	renderCluster(w, s.Cluster)
	renderNodes(w, s.Nodes)
	renderPods(w, s.Pods)
	renderWorkloads(w, s.Workloads)
	renderEvents(w, s.Events)
	renderK3s(w, s.K3s)
	w("top pods: %s reporting=%d", s.TopPods.Avail.State, s.TopPods.Reporting)
	for _, p := range s.TopPods.ByCPU {
		w("  %s %s %s", p.Key, FormatCPU(p.CPUMilli), FormatMemory(p.MemBytes))
	}
	return b.String()
}

type writer func(format string, args ...any)

func avail(a Avail) string {
	if a.State == StateForbidden {
		return "forbidden (" + a.CanI.String() + ")"
	}
	return a.State.String()
}

func renderCluster(w writer, c ClusterPanel) {
	w("cluster: %s", c.Cmd)
	w("  api %s ready=%v %d/%d etcd=%s sev=%s", avail(c.API.Avail), c.API.Ready, c.API.Passed, c.API.Checks, c.API.Etcd, sevWord(c.API.Sev))
	w("  version %s %s distro=%q etcd=%v control-planes=%d", avail(c.Version.Avail), c.Version.GitVersion, c.Version.Distro, c.Version.EmbeddedEtcd, c.Version.ControlPlanes)
	w("  you %s %q", avail(c.You.Avail), c.You.Text)
	w("  age %s %s known=%v", avail(c.Age.Avail), FormatAgo(c.Age.Age), c.Age.Known)
	w("  ns %s %d terminating=%d", avail(c.Namespaces.Avail), c.Namespaces.Count, c.Namespaces.Terminating)
	w("  storage %s %d/%d bound pending=%d lost=%d | %s", avail(c.Storage.Avail), c.Storage.Bound, c.Storage.Total, c.Storage.Pending, c.Storage.Lost, c.Storage.Cmd)
	w("  network %s svc=%d known=%v lb=%d pending=%d %v | %s", avail(c.Network.Avail), c.Network.Services, c.Network.ServicesKnown,
		c.Network.LoadBalancers, c.Network.Pending, c.Network.PendingObjects, c.Network.Cmd)
}

func renderNodes(w writer, n NodesPanel) {
	w("nodes: %s metrics=%s %d/%d ready cpu=%d%% mem=%d%%", avail(n.Avail), n.Metrics.State, n.Ready, n.Total, n.CPU.Percent, n.Memory.Percent)
	for _, r := range n.Rows {
		w("  [%s] %s %v %s cpu=%d%% mem=%d%% pods=%d/%d age=%s | %s", sevWord(r.Sev), r.Key.Name, r.Roles, r.Condition,
			r.CPU.Percent, r.Memory.Percent, r.Pods, r.PodsCap, FormatAgo(r.Age), r.Cmd)
	}
}

func renderPods(w writer, p PodsPanel) {
	ph := p.Phases
	w("pods: %s total=%d running=%d completed=%d waiting=%d failing=%d evicted=%d other=%d notok=%d | %s",
		avail(p.Avail), p.Total, ph.Running, ph.Completed, ph.Waiting, ph.Failing, ph.Evicted, ph.Other, p.NotOK, p.Cmd)
	w("  failing %v", p.Failing)
	w("  per node %v", p.PerNode)
	for _, r := range append(append([]PodRow(nil), p.Problems...), p.Restarted...) {
		w("  [%s] %s %d/%d %s r=%d last=%s age=%s node=%q c=%q init=%v starting=%v | %s", sevWord(r.Sev), r.Key, r.Ready, r.ReadyOf, r.Status,
			r.Restarts, FormatAgo(r.LastRestart), FormatAgo(r.Age), r.Node, r.Container, r.Init, r.Starting, r.Cmd)
	}
}

func renderWorkloads(w writer, p WorkloadsPanel) {
	w("workloads: deploy %d/%d sts %d/%d ds %d/%d | %s", p.Deploy.Ready, p.Deploy.Total, p.STS.Ready, p.STS.Total, p.DS.Ready, p.DS.Total, p.Cmd)
	w("  jobs %s failed=%d active=%d | %s", avail(p.Jobs.Avail), p.Jobs.FailedRecent, p.Jobs.Active, p.Jobs.Cmd)
	w("  cron %s total=%d suspended=%d last=%s | %s", avail(p.Cron.Avail), p.Cron.Total, p.Cron.Suspended, FormatAgo(p.Cron.LastSchedule), p.Cron.Cmd)
	for _, r := range p.Rows {
		w("  [%s] %s %d/%d failed=%v suspended=%v | %s", sevWord(r.Sev), r.Key, r.Ready, r.Desired, r.Failed, r.Suspended, r.Cmd)
	}
}

func renderEvents(w writer, p EventsPanel) {
	w("events: %s total=%d truncated=%v reasons=%d objects=%d counted=%d | %s", avail(p.Avail), p.Total, p.Truncated, p.Reasons, p.Objects, p.Counted, p.Cmd)
	for _, r := range p.Rows {
		w("  %s %s %s c=%q x%d counted=%v | %s", FormatAgo(r.Ago), r.Reason, r.Object, r.Container, r.Count, r.Counted, r.Cmd)
	}
}

func renderK3s(w writer, p K3sPanel) {
	w("k3s: shown=%v hidden=%q | %s", p.Shown, p.Hidden, p.Cmd)
	if !p.Shown {
		return
	}
	for _, c := range p.Components {
		w("  [%s] %s present=%v %d/%d | %s", sevWord(c.Sev), c.Key, c.Present, c.Ready, c.Desired, c.Cmd)
	}
	w("  servicelb %s %d/%d pending=%d | %s", p.ServiceLB.State, p.ServiceLB.Ready, p.ServiceLB.Total, p.ServiceLB.Pending, p.ServiceLB.Cmd)
	w("  helm %s %v failed=%d | %s", avail(p.Helm.Avail), p.Helm.Charts, p.Helm.Failed, p.Helm.Cmd)
	w("  etcd shown=%v reconciled=%v known=%v %s on %q | %s", p.Etcd.Shown, p.Etcd.Reconciled, p.Etcd.Known, FormatAgo(p.Etcd.Ago), p.Etcd.Node, p.Etcd.Cmd)
}

// golden compares got with testdata/<name>.golden, rewriting it under -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from the golden file; run go test -run %s -update to accept\n--- got ---\n%s", path, t.Name(), got)
	}
}
