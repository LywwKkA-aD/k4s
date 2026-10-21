package dashboard_test

import (
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

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
