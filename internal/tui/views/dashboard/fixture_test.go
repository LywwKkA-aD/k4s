package dashboard_test

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Synthetic fixtures shaped like the spec's busy k3s cluster (§3) with the
// critique's corrections: traefik is a Deployment and ServiceLB a svclb-*
// DaemonSet. No real cluster data is committed.

const (
	day    = 24 * time.Hour
	hour   = time.Hour
	minute = time.Minute
)

var start = time.Date(2026, 10, 21, 14, 2, 0, 0, time.UTC)

func ok() dash.Avail { return dash.Avail{State: dash.StateOK} }

func kc(args ...string) dash.Cmd { return dash.Cmd{Args: args} }

func approxCmd(args ...string) dash.Cmd { return dash.Cmd{Args: args, Approx: true} }

func podKey(ns, name string) dash.ObjectKey {
	return dash.ObjectKey{Kind: "pod", Namespace: ns, Name: name}
}

func meter(used, capacity int64, sev dash.Severity) dash.Meter {
	return dash.Meter{Known: true, Used: used, Capacity: capacity, Percent: int(used * 100 / capacity), Sev: sev}
}

func node(name string, roles []string, cpu, mem dash.Meter, pods, podsCap int, age time.Duration, sev dash.Severity) dash.NodeRow {
	k := dash.ObjectKey{Kind: "node", Name: name}
	return dash.NodeRow{
		Key: k, Roles: roles, Condition: "Ready", Sev: sev, CPU: cpu, Memory: mem,
		Pods: pods, PodsCap: podsCap, PodsKnown: true, Age: age, Kubelet: "v1.36.3+k3s1",
		Cmd: kc("describe", "node", name),
	}
}

func problem(ns, name, status string, ready, of, restarts int, last, age time.Duration, nodeName string, sev dash.Severity) dash.PodRow {
	return dash.PodRow{
		Key: podKey(ns, name), Resolved: true, Ready: ready, ReadyOf: of, ReadyKnown: true,
		Status: status, Sev: sev, Restarts: restarts, LastRestart: last, HasRestart: restarts > 0,
		Age: age, Node: nodeName, Cmd: kc("describe", "pod", name, "-n", ns),
	}
}

// busyK3s is the 207x58 mockup's cluster after one refresh.
func busyK3s() dash.Snapshot {
	var s dash.Snapshot
	s.At = start
	s.Caps = dash.Caps{Metrics: true, K3s: true, Etcd: true}
	for _, e := range dash.Endpoints() {
		s.Ledger[e] = dash.Usage{Fetched: true, State: dash.StateOK}
	}
	s.Cluster = busyCluster()
	s.Nodes = busyNodes()
	s.Pods = busyPods()
	s.Workloads = busyWorkloads()
	s.Events = busyEvents()
	s.K3s = busyK3sPanel()
	s.TopPods = busyTopPods()
	s.Verdict = dash.Verdict{
		Focus: dash.PanelPods, Cmd: approxCmd("get", "nodes,pods,deploy,sts,ds", "-A"),
		Chips: []dash.Chip{
			{Sev: dash.SevFailing, Panel: dash.PanelPods, Count: 3, Text: "3 CrashLoopBackOff"},
			{Sev: dash.SevFailing, Panel: dash.PanelPods, Count: 1, Text: "1 ImagePullBackOff"},
			{Sev: dash.SevFailing, Panel: dash.PanelWorkloads, Count: 4, Text: "4 workloads not ready"},
			{Sev: dash.SevFailing, Panel: dash.PanelWorkloads, Count: 1, Text: "1 job failed"},
			{Sev: dash.SevWaiting, Panel: dash.PanelNodes, Count: 1, Text: "k3s-agent-1 mem 87%"},
			{Sev: dash.SevWaiting, Panel: dash.PanelPods, Count: 2, Text: "2 not ready"},
			{Sev: dash.SevWaiting, Panel: dash.PanelEvents, Count: 1, Text: "1 warning in 10m"},
		},
	}
	s.Next = []dash.Suggestion{
		{Cmd: kc("logs", "payments-worker-6c9f7d8b5-xk2lp", "-n", "shop", "-c", "worker", "--previous"), Question: "why does it keep crashing?", Action: dash.ActionLogsPrevious, Target: podKey("shop", "payments-worker-6c9f7d8b5-xk2lp"), Container: "worker", Sev: dash.SevFailing},
		{Cmd: kc("describe", "pod", "api-7f6d9c4b8-2mzqt", "-n", "staging"), Question: "why can't it pull its image?", Action: dash.ActionDescribe, Target: podKey("staging", "api-7f6d9c4b8-2mzqt"), Sev: dash.SevFailing},
		{Cmd: kc("logs", "grafana-5d8f7c6b9-wq2lx", "-n", "monitoring", "-c", "grafana"), Question: "why does it fail its probe?", Action: dash.ActionLogs, Target: podKey("monitoring", "grafana-5d8f7c6b9-wq2lx"), Container: "grafana", Sev: dash.SevWaiting},
		{Cmd: kc("describe", "job", "nightly-e2e-28741000", "-n", "ci"), Question: "why did the job fail?", Action: dash.ActionRunner, Target: dash.ObjectKey{Kind: "job", Namespace: "ci", Name: "nightly-e2e-28741000"}, Sev: dash.SevFailing},
		{Cmd: kc("describe", "svc", "traefik", "-n", "kube-system"), Question: "why is the LoadBalancer pending?", Action: dash.ActionDescribe, Target: dash.ObjectKey{Kind: "service", Namespace: "kube-system", Name: "traefik"}, Sev: dash.SevInfo},
	}
	s.Daily = daily()
	return s
}

// daily is the command of the day the fixtures carry.
func daily() dash.Suggestion {
	return dash.Suggestion{
		Cmd:      kc("get", "pods", "-A", "--field-selector=status.phase!=Running,status.phase!=Succeeded"),
		Question: "which pods are neither running nor done?", Action: dash.ActionCopy, Target: dash.ObjectKey{Kind: "daily"},
	}
}

func busyCluster() dash.ClusterPanel {
	return dash.ClusterPanel{
		Cmd:        kc("get", "--raw=/readyz?verbose"),
		API:        dash.APIRow{Avail: ok(), Ready: true, Checks: 8, Passed: 8, Etcd: dash.StateOK, Cmd: kc("get", "--raw=/readyz?verbose")},
		Version:    dash.VersionRow{Avail: ok(), GitVersion: "v1.36.3+k3s1", Distro: "k3s", EmbeddedEtcd: true, ControlPlanes: 1, Cmd: kc("version")},
		You:        dash.TextRow{Avail: ok(), Text: "devops-friend", Cmd: kc("auth", "whoami")},
		Age:        dash.AgeRow{Avail: ok(), Age: 212 * day, Known: true, Cmd: kc("get", "ns", "kube-system")},
		Namespaces: dash.NamespacesRow{Avail: ok(), Count: 33, Cmd: kc("get", "ns")},
		Storage:    dash.StorageRow{Avail: ok(), Total: 26, Bound: 26, Cmd: kc("get", "pvc", "-A")},
		Network: dash.NetworkRow{
			Avail: ok(), Services: 102, ServicesKnown: true, LoadBalancers: 2, Pending: 1,
			Cmd: kc("get", "svc", "-A", "--field-selector", "spec.type=LoadBalancer"),
		},
		Server: dash.TextRow{Avail: ok(), Text: "https://10.0.0.5:6443", Cmd: kc("cluster-info")},
	}
}

func busyNodes() dash.NodesPanel {
	gi := int64(1 << 30)
	return dash.NodesPanel{
		Avail: ok(), Metrics: ok(), Cmd: kc("get", "nodes", "-o", "wide"), TopCmd: approxCmd("top", "nodes"),
		Total: 3, Ready: 3,
		CPU:    meter(12900, 36000, dash.SevOK),
		Memory: meter(131*gi, 192*gi, dash.SevOK),
		Rows: []dash.NodeRow{
			node("k3s-cp-1", []string{"control-plane", "etcd"}, meter(1520, 4000, dash.SevOK), meter(11*gi+gi/3, 16*gi, dash.SevOK), 112, 250, 212*day, dash.SevOK),
			node("k3s-agent-1", nil, meter(2080, 4000, dash.SevOK), meter(7*gi, 8*gi, dash.SevWaiting), 41, 110, 212*day, dash.SevWaiting),
			node("k3s-agent-2", nil, meter(560, 4000, dash.SevOK), meter(3*gi, 8*gi, dash.SevOK), 32, 110, 98*day, dash.SevOK),
		},
	}
}

func busyPods() dash.PodsPanel {
	evicted := problem("growlytics", "report-28741230-9q8wd", "Evicted", 0, 1, 0, 0, 48*minute, "k3s-agent-1", dash.SevMinor)
	traefik := problem("kube-system", "traefik-6f5f87584-x7k2p", "Running", 1, 1, 7, day, 12*day, "k3s-cp-1", dash.SevInfo)
	ingest := problem("growlytics", "ingest-6d8c7b5f9-m2kzp", "Running", 1, 1, 3, 5*hour, 2*day, "k3s-agent-2", dash.SevInfo)
	return dash.PodsPanel{
		Avail: ok(), Cmd: kc("get", "pods", "-A"), Total: 185, NotOK: 7,
		Phases:  dash.PhaseCounts{Running: 171, Completed: 7, Waiting: 2, Failing: 4, Evicted: 1},
		Failing: []dash.ReasonCount{{Reason: "CrashLoopBackOff", Count: 3}, {Reason: "ImagePullBackOff", Count: 1}},
		PerNode: []dash.NodeCount{{Node: "k3s-agent-1", Pods: 41}, {Node: "k3s-agent-2", Pods: 32}, {Node: "k3s-cp-1", Pods: 112}},
		Problems: []dash.PodRow{
			problem("shop", "payments-worker-6c9f7d8b5-xk2lp", "CrashLoopBackOff", 0, 1, 41, 2*minute, 3*hour, "k3s-agent-1", dash.SevFailing),
			problem("shop", "payments-worker-6c9f7d8b5-r8vqn", "CrashLoopBackOff", 0, 1, 39, 2*minute, 3*hour, "k3s-agent-2", dash.SevFailing),
			problem("monitoring", "loki-backend-0", "CrashLoopBackOff", 1, 2, 12, 0, 5*day, "k3s-cp-1", dash.SevFailing),
			problem("staging", "api-7f6d9c4b8-2mzqt", "ImagePullBackOff", 0, 1, 0, 0, 22*minute, "k3s-agent-1", dash.SevFailing),
			problem("monitoring", "grafana-5d8f7c6b9-wq2lx", "Running", 0, 1, 0, 0, 9*minute, "k3s-cp-1", dash.SevWaiting),
			problem("ci", "runner-build-8812-k7x2p", "Pending", 0, 1, 0, 0, 6*minute, "", dash.SevWaiting),
			evicted,
		},
		Restarted: []dash.PodRow{traefik, ingest},
	}
}
