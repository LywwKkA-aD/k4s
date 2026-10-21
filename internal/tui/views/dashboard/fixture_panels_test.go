package dashboard_test

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Panels 4 and 5 of the busy fixture, shaped like the 207x58 mockup.

func workload(kind, ns, name string, ready, desired, upToDate, avail int, sev dash.Severity) dash.WorkloadRow {
	k := dash.ObjectKey{Kind: kind, Namespace: ns, Name: name}
	return dash.WorkloadRow{
		Key: k, Resolved: true, Ready: ready, Desired: desired, UpToDate: upToDate, Available: avail,
		Sev: sev, Cmd: kc("describe", kind, name, "-n", ns),
	}
}

func busyWorkloads() dash.WorkloadsPanel {
	job := workload("job", "ci", "nightly-e2e-28741000", 0, 0, 0, 0, dash.SevFailing)
	job.Failed, job.FailedAgo = true, 41*minute
	cron := workload("cronjob", "growlytics", "report-weekly", 0, 0, 0, 0, dash.SevMinor)
	cron.Suspended = true
	return dash.WorkloadsPanel{
		Cmd:    kc("get", "deploy,sts,ds", "-A"),
		Deploy: dash.KindSummary{Avail: ok(), Ready: 49, Total: 52},
		STS:    dash.KindSummary{Avail: ok(), Ready: 7, Total: 8},
		DS:     dash.KindSummary{Avail: ok(), Ready: 5, Total: 5},
		Jobs: dash.JobsSummary{
			Avail: ok(), FailedRecent: 1, Active: 2,
			Cmd: kc("get", "jobs", "-A", "--field-selector", "status.successful=0"),
		},
		Cron: dash.CronSummary{
			Avail: ok(), Total: 14, Suspended: 1, LastSchedule: 3 * minute, HasLast: true,
			Cmd: kc("get", "cronjobs", "-A"),
		},
		Rows: []dash.WorkloadRow{
			workload("deployment", "shop", "payments-worker", 0, 2, 2, 0, dash.SevFailing),
			workload("deployment", "staging", "api", 0, 1, 1, 0, dash.SevFailing),
			workload("deployment", "monitoring", "grafana", 0, 1, 1, 0, dash.SevFailing),
			workload("statefulset", "monitoring", "loki-backend", 0, 1, 0, 0, dash.SevFailing),
			job,
			cron,
		},
	}
}

func warning(kind, ns, name, reason, message string, count int32, ago time.Duration, counted bool) dash.EventRow {
	k := dash.ObjectKey{Kind: kind, Namespace: ns, Name: name}
	return dash.EventRow{
		Object: k, Reason: reason, Message: message, Count: count, Ago: ago,
		Counted: counted, Flagged: !counted, Cmd: kc("describe", kind, name, "-n", ns),
	}
}

func busyEvents() dash.EventsPanel {
	return dash.EventsPanel{
		Avail: ok(), Cmd: kc("events", "-A", "--types=Warning"), Total: 10, Reasons: 7, Objects: 9, Counted: 1,
		LastHour: [30]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 1, 0, 0, 1, 2, 4, 5, 4, 3, 4, 5, 6, 5, 6},
		Rows: []dash.EventRow{
			warning("pod", "monitoring", "loki-backend-0", "BackOff", "Back-off restarting failed container ingester in pod loki-backend-0_monitoring(5c1d)", 112, 0, false),
			warning("pod", "shop", "payments-worker-6c9f7d8b5-xk2lp", "BackOff", "Back-off restarting failed container worker in pod payments-worker-6c9f7d8b5-xk2lp_shop(9e2a)", 41, 2*minute, false),
			warning("pod", "shop", "payments-worker-6c9f7d8b5-r8vqn", "BackOff", "Back-off restarting failed container worker in pod payments-worker-6c9f7d8b5-r8vqn_shop(41b7)", 39, 2*minute, false),
			warning("pod", "growlytics", "backup-7d9f8c6b5-q2w4e", "FailedMount", `MountVolume.SetUp failed for volume "creds" : secret "backup-creds" not found`, 3, 4*minute, true),
			warning("pod", "ci", "runner-build-8812-k7x2p", "FailedScheduling", "0/3 nodes are available: 3 Insufficient memory. preemption: 0/3 nodes are available: 3 No preemption victims found for incoming pod.", 4, 6*minute, false),
			warning("pod", "monitoring", "grafana-5d8f7c6b9-wq2lx", "Unhealthy", "Readiness probe failed: HTTP probe failed with statuscode: 503", 27, 9*minute, false),
			warning("pod", "staging", "api-7f6d9c4b8-2mzqt", "Failed", `Failed to pull image "registry.example.internal/staging/api:2.4.1": not found`, 9, 22*minute, false),
			warning("pod", "staging", "api-7f6d9c4b8-2mzqt", "BackOff", `Back-off pulling image "registry.example.internal/staging/api:2.4.1"`, 86, 22*minute, false),
			warning("job", "ci", "nightly-e2e-28741000", "BackoffLimitExceeded", "Job has reached the specified backoff limit", 1, 41*minute, false),
			warning("pod", "growlytics", "report-28741230-9q8wd", "Evicted", "The node was low on resource: memory. Threshold quantity: 100Mi, available: 81232Ki.", 1, 48*minute, false),
		},
	}
}
