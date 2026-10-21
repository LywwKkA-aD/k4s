package dash

import (
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

const jobWindow = time.Hour

// deployRowsFrom reads the deployments Table: READY is "a/b", UP-TO-DATE
// and AVAILABLE are integers.
func deployRowsFrom(t table, ns string) []workloadRow {
	name, ready := t.col("Name"), t.col("Ready")
	upToDate, available := t.col("Up-to-date"), t.col("Available")
	out := make([]workloadRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		r := workloadRow{name: n, namespace: ns}
		if s, ok := t.str(i, ready); ok {
			r.ready, r.desired, r.readyOK = parseReady(s)
		}
		u, _ := t.num(i, upToDate)
		a, _ := t.num(i, available)
		r.upToDate, r.available = u, a
		out = append(out, r)
	}
	return out
}

// stsRowsFrom reads the statefulsets Table, whose READY is "a/b".
func stsRowsFrom(t table, ns string) []workloadRow {
	return deployRowsFrom(t, ns)
}

// dsRowsFrom reads the daemonsets Table: DESIRED and READY are integers.
func dsRowsFrom(t table, ns string) []workloadRow {
	name, desired, ready, available := t.col("Name"), t.col("Desired"), t.col("Ready"), t.col("Available")
	out := make([]workloadRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		d, okD := t.num(i, desired)
		r, okR := t.num(i, ready)
		a, _ := t.num(i, available)
		out = append(out, workloadRow{
			name: n, namespace: ns, desired: d, ready: r, available: a, readyOK: okD && okR,
		})
	}
	return out
}

// cronRowsFrom reads the cronjobs Table: SUSPEND is a bool, LAST SCHEDULE
// a relative time.
func cronRowsFrom(t table, ns string, now time.Time) []cronRow {
	name, suspend, last := t.col("Name"), t.col("Suspend"), t.col("Last Schedule")
	out := make([]cronRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		r := cronRow{name: n, namespace: ns}
		r.suspend, _ = t.flag(i, suspend)
		if s, ok := t.str(i, last); ok {
			if d, ok := parseAge(s); ok {
				r.lastSchedule = now.Add(-d)
			}
		}
		out = append(out, r)
	}
	return out
}

// jobsFrom keeps what the jobs line needs from a status.successful=0 list.
// The selector is re-applied here: a fake clientset ignores it.
func jobsFrom(items []batchv1.Job) []jobInfo {
	out := make([]jobInfo, 0, len(items))
	for _, j := range items {
		if j.Status.Succeeded > 0 {
			continue
		}
		info := jobInfo{name: j.Name, namespace: j.Namespace, active: j.Status.Active}
		for _, c := range j.Status.Conditions {
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				info.failed, info.failedAt = true, c.LastTransitionTime.Time
			}
		}
		out = append(out, info)
	}
	return out
}

func deriveWorkloads(st store, led Ledger, scope Scope, now time.Time) WorkloadsPanel {
	ns := scope.Namespace
	p := WorkloadsPanel{
		Cmd:    withScope(ns, "get", "deploy,sts,ds"),
		Deploy: kindSummary(st.deploys, availOf(Deployments, led)),
		STS:    kindSummary(st.sts, availOf(StatefulSets, led)),
		DS:     kindSummary(st.ds, availOf(DaemonSets, led)),
		Jobs: JobsSummary{
			Avail: availOf(Jobs, led),
			Cmd:   withScope(ns, "get", "jobs", "--field-selector", "status.successful=0"),
		},
		Cron: CronSummary{Avail: availOf(CronJobs, led), Cmd: withScope(ns, "get", "cronjobs"), Total: len(st.cronjobs)},
	}
	for _, kr := range workloadKinds(st) {
		p.Rows = append(p.Rows, notReadyRows(st, kr.kind, kr.rows)...)
	}
	for _, j := range st.jobs {
		if j.active > 0 {
			p.Jobs.Active++
		}
		if !j.failed || now.Sub(j.failedAt) >= jobWindow {
			continue
		}
		p.Jobs.FailedRecent++
		key := ObjectKey{Kind: "job", Namespace: j.namespace, Name: j.name}
		p.Rows = append(p.Rows, WorkloadRow{
			Key: key, Resolved: true, Sev: SevFailing, Failed: true, FailedAgo: since(now, j.failedAt),
			Cmd: describe("job", j.name, j.namespace),
		})
	}
	p.Cron, p.Rows = cronSummary(st, p.Cron, p.Rows, now)
	sortWorkloadRows(p.Rows)
	return p
}

func kindSummary(rows []workloadRow, a Avail) KindSummary {
	s := KindSummary{Avail: a, Total: len(rows)}
	for _, r := range rows {
		if workloadSeverity(r.ready, r.desired) == SevOK {
			s.Ready++
		}
	}
	return s
}

// kindRows pairs a kubectl kind word with its Table rows.
type kindRows struct {
	kind string
	rows []workloadRow
}

func workloadKinds(st store) []kindRows {
	return []kindRows{{"deployment", st.deploys}, {"statefulset", st.sts}, {"daemonset", st.ds}}
}

func notReadyRows(st store, kind string, rows []workloadRow) []WorkloadRow {
	var out []WorkloadRow
	used := map[string][]bool{}
	for _, r := range rows {
		sev := workloadSeverity(r.ready, r.desired)
		if sev == SevOK {
			continue
		}
		key, resolved := placeWorkload(st, kind, r, used)
		out = append(out, WorkloadRow{
			Key: key, Resolved: resolved, Ready: r.ready, Desired: r.desired, UpToDate: r.upToDate,
			Available: r.available, Sev: sev, Cmd: workloadCmd(key, resolved),
		})
	}
	return out
}

// placeWorkload gives a Table row its namespace from the list it came from
// or from the cached lookup.
func placeWorkload(st store, kind string, r workloadRow, used map[string][]bool) (ObjectKey, bool) {
	key := ObjectKey{Kind: kind, Namespace: r.namespace, Name: r.name}
	if r.namespace != "" {
		return key, true
	}
	e, ok := st.resolved[resolveKey{kind, r.name}]
	if !ok {
		return key, false
	}
	if used[r.name] == nil {
		used[r.name] = make([]bool, len(e.objs))
	}
	o, i, ok := matchWorkload(e.objs, r, used[r.name])
	if !ok {
		return key, false
	}
	used[r.name][i] = true
	key.Namespace = o.namespace
	return key, true
}

func workloadCmd(key ObjectKey, resolved bool) Cmd {
	if resolved {
		return describe(key.Kind, key.Name, key.Namespace)
	}
	return kubectl("get", key.Kind, "-A", "--field-selector", "metadata.name="+key.Name)
}

func cronSummary(st store, c CronSummary, rows []WorkloadRow, now time.Time) (CronSummary, []WorkloadRow) {
	used := map[string][]bool{}
	var newest time.Time
	for _, r := range st.cronjobs {
		if r.lastSchedule.After(newest) {
			newest = r.lastSchedule
		}
		if !r.suspend {
			continue
		}
		c.Suspended++
		key, resolved := placeWorkload(st, "cronjob", workloadRow{name: r.name, namespace: r.namespace}, used)
		rows = append(rows, WorkloadRow{Key: key, Resolved: resolved, Sev: SevMinor, Suspended: true, Cmd: workloadCmd(key, resolved)})
	}
	if !newest.IsZero() {
		c.LastSchedule, c.HasLast = since(now, newest), true
	}
	return c, rows
}

var kindOrder = map[string]int{"deployment": 0, "statefulset": 1, "daemonset": 2, "job": 3, "cronjob": 4}

func sortWorkloadRows(rows []WorkloadRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Sev != b.Sev {
			return a.Sev > b.Sev
		}
		if kindOrder[a.Key.Kind] != kindOrder[b.Key.Kind] {
			return kindOrder[a.Key.Kind] < kindOrder[b.Key.Kind]
		}
		return a.Key.String() < b.Key.String()
	})
}
