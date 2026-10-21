package dashboard_test

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Synthetic fixtures for the degraded states (§3 degraded mockup, §10.5).

func stale(d time.Duration) dash.Avail { return dash.Avail{State: dash.StateStale, Stale: d} }

func absent() dash.Avail { return dash.Avail{State: dash.StateAbsent} }

// nsScopedEKS is a user who may list only namespace shop, on EKS without
// metrics-server: -A is denied, so every namespaced list fell back to the
// context's namespace.
func nsScopedEKS() dash.Snapshot {
	var s dash.Snapshot
	s.At = start
	s.Scope = dash.Scope{Namespace: "shop", Context: "shop", Fallback: true}
	for _, e := range dash.Endpoints() {
		s.Ledger[e] = dash.Usage{Fetched: true, State: dash.StateOK, Answered: true}
	}
	for _, e := range []dash.Endpoint{dash.Nodes, dash.Namespaces} {
		s.Ledger[e] = dash.Usage{Fetched: true, State: dash.StateForbidden}
	}
	s.Ledger[dash.NodeMetrics] = dash.Usage{Fetched: true, State: dash.StateAbsent}
	s.Cluster = dash.ClusterPanel{
		Cmd:        kc("get", "--raw=/readyz?verbose"),
		API:        dash.APIRow{Avail: ok(), Ready: true, Cmd: kc("get", "--raw=/readyz?verbose")},
		Version:    dash.VersionRow{Avail: ok(), GitVersion: "v1.35.4-eks-3025e55", Distro: "eks", Cmd: kc("version")},
		You:        dash.TextRow{Avail: ok(), Text: "shop-dev", Cmd: kc("auth", "whoami")},
		Age:        dash.AgeRow{Avail: denied("auth", "can-i", "get", "namespaces")},
		Namespaces: dash.NamespacesRow{Avail: denied("auth", "can-i", "list", "namespaces"), Cmd: kc("get", "ns")},
		Storage:    dash.StorageRow{Avail: ok(), Total: 4, Bound: 4, Cmd: kc("get", "pvc", "-n", "shop")},
		Network: dash.NetworkRow{
			Avail: ok(), Services: 12, ServicesKnown: true,
			Cmd: kc("get", "svc", "-n", "shop", "--field-selector", "spec.type=LoadBalancer"),
		},
		Server: dash.TextRow{Avail: ok(), Text: "https://C0FFEE.gr7.eu-west-1.eks.amazonaws.com", Cmd: kc("cluster-info")},
	}
	s.Nodes = dash.NodesPanel{
		Avail: denied("auth", "can-i", "list", "nodes"), Metrics: absent(),
		Cmd: kc("get", "nodes", "-o", "wide"), TopCmd: approxCmd("top", "nodes"),
	}
	s.Pods = dash.PodsPanel{
		Avail: ok(), Cmd: kc("get", "pods", "-n", "shop"), Scope: "shop", Total: 41,
		Phases:  dash.PhaseCounts{Running: 41},
		PerNode: []dash.NodeCount{{Node: "ip-10-0-1-17", Pods: 22}, {Node: "ip-10-0-2-41", Pods: 19}},
	}
	s.Workloads = dash.WorkloadsPanel{
		Cmd:    kc("get", "deploy,sts,ds", "-n", "shop"),
		Deploy: dash.KindSummary{Avail: ok(), Ready: 9, Total: 9},
		STS:    dash.KindSummary{Avail: ok()},
		DS:     dash.KindSummary{Avail: ok()},
		Jobs:   dash.JobsSummary{Avail: ok(), Cmd: kc("get", "jobs", "-n", "shop", "--field-selector", "status.successful=0")},
		Cron:   dash.CronSummary{Avail: ok(), Cmd: kc("get", "cronjobs", "-n", "shop")},
	}
	s.Events = dash.EventsPanel{Avail: ok(), Cmd: kc("events", "-n", "shop", "--types=Warning")}
	s.K3s = dash.K3sPanel{Hidden: "not k3s"}
	s.TopPods = dash.TopPodsPanel{Avail: absent(), CmdCPU: kc("top", "pods", "-n", "shop", "--sort-by=cpu")}
	s.Verdict = dash.Verdict{Partial: true, Cmd: approxCmd("get", "nodes,pods,deploy,sts,ds", "-n", "shop")}
	s.Next = []dash.Suggestion{
		{Cmd: kc("auth", "can-i", "--list", "-n", "shop"), Question: "what am I allowed to see here?", Action: dash.ActionCopy, Target: dash.ObjectKey{Kind: "access"}, Sev: dash.SevWaiting},
		{Cmd: kc("api-resources", "--api-group=metrics.k8s.io"), Question: "is the metrics API served?", Action: dash.ActionCopy, Target: dash.ObjectKey{Kind: "metrics"}, Sev: dash.SevInfo},
	}
	return s
}

// noMetricsK3s is the busy cluster on a k3s started with
// --disable=metrics-server: discovery answers without metrics.k8s.io.
func noMetricsK3s() dash.Snapshot {
	s := busyK3s()
	s.Caps.Metrics = false
	s.Ledger[dash.NodeMetrics] = dash.Usage{Fetched: true, State: dash.StateAbsent}
	s.Ledger[dash.PodMetrics] = dash.Usage{}
	s.Nodes.Metrics = absent()
	s.Nodes.CPU, s.Nodes.Memory = dash.Meter{}, dash.Meter{}
	for i := range s.Nodes.Rows {
		s.Nodes.Rows[i].CPU, s.Nodes.Rows[i].Memory = dash.Meter{}, dash.Meter{}
		s.Nodes.Rows[i].Sev = dash.SevOK
	}
	s.TopPods = dash.TopPodsPanel{Avail: absent(), CmdCPU: s.TopPods.CmdCPU}
	chips := make([]dash.Chip, 0, len(s.Verdict.Chips))
	for _, c := range s.Verdict.Chips {
		if c.Panel != dash.PanelNodes {
			chips = append(chips, c)
		}
	}
	s.Verdict.Chips = chips
	s.Next = append(s.Next, dash.Suggestion{
		Cmd: kc("api-resources", "--api-group=metrics.k8s.io"), Question: "is the metrics API served?", Action: dash.ActionCopy,
		Target: dash.ObjectKey{Kind: "metrics"}, Sev: dash.SevInfo,
	})
	return s
}
