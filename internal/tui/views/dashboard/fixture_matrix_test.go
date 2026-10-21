package dashboard_test

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// vanillaKind is a three-node kind cluster as it comes: upstream
// Kubernetes, no metrics-server and no k3s, kindnet and kube-proxy as
// DaemonSets, one pod still pulling its image.
func vanillaKind() dash.Snapshot {
	var s dash.Snapshot
	s.At = start
	for _, e := range dash.Endpoints() {
		s.Ledger[e] = dash.Usage{Fetched: true, State: dash.StateOK, Answered: true}
	}
	s.Ledger[dash.NodeMetrics] = dash.Usage{Fetched: true, State: dash.StateAbsent}
	s.Ledger[dash.PodMetrics] = dash.Usage{}
	s.Cluster = dash.ClusterPanel{
		Cmd:        kc("get", "--raw=/readyz?verbose"),
		API:        dash.APIRow{Avail: ok(), Ready: true, Checks: 9, Passed: 9, Etcd: dash.StateOK, Cmd: kc("get", "--raw=/readyz?verbose")},
		Version:    dash.VersionRow{Avail: ok(), GitVersion: "v1.35.1", ControlPlanes: 1, EmbeddedEtcd: true, Cmd: kc("version")},
		You:        dash.TextRow{Avail: ok(), Text: "kubernetes-admin", Cmd: kc("auth", "whoami")},
		Age:        dash.AgeRow{Avail: ok(), Age: 3 * hour, Known: true, Cmd: kc("get", "ns", "kube-system")},
		Namespaces: dash.NamespacesRow{Avail: ok(), Count: 6, Cmd: kc("get", "ns")},
		Storage:    dash.StorageRow{Avail: ok(), Cmd: kc("get", "pvc", "-A")},
		Network: dash.NetworkRow{
			Avail: ok(), Services: 4, ServicesKnown: true,
			Cmd: kc("get", "svc", "-A", "--field-selector", "spec.type=LoadBalancer"),
		},
		Server: dash.TextRow{Avail: ok(), Text: "https://127.0.0.1:39117", Cmd: kc("cluster-info")},
	}
	kindNode := func(name string, roles []string, pods int) dash.NodeRow {
		n := node(name, roles, dash.Meter{}, dash.Meter{}, pods, 110, 3*hour, dash.SevOK)
		n.Kubelet, n.Etcd, n.ControlPlane = "v1.35.1", len(roles) > 0, len(roles) > 0
		return n
	}
	s.Nodes = dash.NodesPanel{
		Avail: ok(), Metrics: absent(), Cmd: kc("get", "nodes", "-o", "wide"), TopCmd: approxCmd("top", "nodes"),
		Total: 3, Ready: 3,
		Rows: []dash.NodeRow{
			kindNode("kind-control-plane", []string{"control-plane"}, 9),
			kindNode("kind-worker", nil, 6),
			kindNode("kind-worker2", nil, 5),
		},
	}
	pull := problem("default", "web-5d4f8b7c9-q8x2m", "ContainerCreating", 0, 1, 0, 0, 40*time.Second, "kind-worker2", dash.SevWaiting)
	pull.Starting = true
	s.Pods = dash.PodsPanel{
		Avail: ok(), Cmd: kc("get", "pods", "-A"), Total: 20, NotOK: 1,
		Phases:   dash.PhaseCounts{Running: 19, Waiting: 1},
		PerNode:  []dash.NodeCount{{Node: "kind-control-plane", Pods: 9}, {Node: "kind-worker", Pods: 6}, {Node: "kind-worker2", Pods: 5}},
		Problems: []dash.PodRow{pull},
	}
	s.Workloads = dash.WorkloadsPanel{
		Cmd:    kc("get", "deploy,sts,ds", "-A"),
		Deploy: dash.KindSummary{Avail: ok(), Ready: 3, Total: 4},
		STS:    dash.KindSummary{Avail: ok()},
		DS:     dash.KindSummary{Avail: ok(), Ready: 2, Total: 2},
		Jobs:   dash.JobsSummary{Avail: ok(), Cmd: kc("get", "jobs", "-A", "--field-selector", "status.successful=0")},
		Cron:   dash.CronSummary{Avail: ok(), Cmd: kc("get", "cronjobs", "-A")},
		Rows:   []dash.WorkloadRow{workload("deployment", "default", "web", 0, 1, 1, 0, dash.SevWaiting)},
	}
	s.Events = dash.EventsPanel{Avail: ok(), Cmd: kc("events", "-A", "--types=Warning")}
	s.K3s = dash.K3sPanel{Hidden: "not k3s", Cmd: kc("get", "deploy,ds", "-n", "kube-system")}
	s.TopPods = dash.TopPodsPanel{Avail: absent(), CmdCPU: kc("top", "pods", "-A", "--sort-by=cpu")}
	s.Verdict = dash.Verdict{
		Focus: dash.PanelWorkloads, Cmd: approxCmd("get", "nodes,pods,deploy,sts,ds", "-A"),
		Chips: []dash.Chip{{Sev: dash.SevWaiting, Panel: dash.PanelWorkloads, Count: 1, Text: "1 workload not ready"}},
	}
	s.Next = []dash.Suggestion{
		{Cmd: kc("describe", "deployment", "web", "-n", "default"), Question: "why is the rollout stuck?", Action: dash.ActionDescribe,
			Target: dash.ObjectKey{Kind: "deployment", Namespace: "default", Name: "web"}, Sev: dash.SevWaiting},
		{Cmd: kc("api-resources", "--api-group=metrics.k8s.io"), Question: "is the metrics API served?", Action: dash.ActionCopy,
			Target: dash.ObjectKey{Kind: "metrics"}, Sev: dash.SevInfo},
	}
	s.Daily = daily()
	return s
}

// allClear is the busy cluster with nothing to fix: every pod running or
// done, every workload ready, no node hot. Its warnings are the routine
// ones a real k3s keeps (kube-system probe flaps on pods that have since
// recovered), which the data layer does not count toward the verdict
// (critique: warnings are △ at most, and only on objects still in trouble).
func allClear() dash.Snapshot {
	s := busyK3s()
	gi := int64(1 << 30)
	s.Nodes.Rows[1].Memory = meter(5*gi, 8*gi, dash.SevOK)
	s.Nodes.Rows[1].Sev = dash.SevOK
	s.Pods = dash.PodsPanel{
		Avail: ok(), Cmd: kc("get", "pods", "-A"), Total: 185,
		Phases:  dash.PhaseCounts{Running: 178, Completed: 7},
		PerNode: s.Pods.PerNode,
	}
	s.Workloads = dash.WorkloadsPanel{
		Cmd:    s.Workloads.Cmd,
		Deploy: dash.KindSummary{Avail: ok(), Ready: 52, Total: 52},
		STS:    dash.KindSummary{Avail: ok(), Ready: 8, Total: 8},
		DS:     dash.KindSummary{Avail: ok(), Ready: 5, Total: 5},
		Jobs:   dash.JobsSummary{Avail: ok(), Cmd: s.Workloads.Jobs.Cmd},
		Cron:   dash.CronSummary{Avail: ok(), Total: 14, LastSchedule: 3 * minute, HasLast: true, Cmd: s.Workloads.Cron.Cmd},
	}
	probe := warning("pod", "kube-system", "coredns-ccb96694c-8xq2z", "Unhealthy",
		"Readiness probe failed: HTTP probe failed with statuscode: 503", 2, 38*minute, false)
	probe.Flagged = false
	dns := warning("pod", "kube-system", "svclb-traefik-4f9d2-hx7kq", "DNSConfigForming",
		"Nameserver limits were exceeded, some nameservers have been omitted", 14, 51*minute, false)
	dns.Flagged = false
	s.Events = dash.EventsPanel{
		Avail: ok(), Cmd: s.Events.Cmd, Total: 2, Reasons: 2, Objects: 2,
		LastHour: [30]int{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Rows:     []dash.EventRow{probe, dns},
	}
	s.Cluster.Network.Pending = 0
	s.Verdict = dash.Verdict{AllClear: true, Cmd: s.Verdict.Cmd}
	s.Next = nil
	return s
}
