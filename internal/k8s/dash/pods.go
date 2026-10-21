package dash

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxRestartedRows = 3
	restartWindow    = 24 * time.Hour
)

// podColumns are the pods Table columns the panel reads, by name.
var podColumns = []string{"Name", "Ready", "Status", "Restarts", "Age", "Node"}

// podRowsFrom decodes a pods Table. ns is the namespace the list was made
// in ("" for all namespaces, where rows carry none). Relative AGE and
// RESTARTS cells become absolute times against now, the fetch time.
func podRowsFrom(t table, ns string, now time.Time) []podRow {
	name, ready, status := t.col("Name"), t.col("Ready"), t.col("Status")
	restarts, age, node := t.col("Restarts"), t.col("Age"), t.col("Node")
	out := make([]podRow, 0, t.len())
	for i := range t.len() {
		n, ok := t.str(i, name)
		if !ok {
			continue
		}
		r := podRow{name: n, namespace: ns}
		if s, ok := t.str(i, ready); ok {
			r.ready, r.readyOf, r.readyOK = parseReady(s)
		}
		r.status, _ = t.str(i, status)
		if s, ok := t.str(i, restarts); ok {
			count, ago, hasAgo, _ := parseRestarts(s)
			r.restarts = count
			if hasAgo {
				r.lastRestart = now.Add(-ago)
			}
		}
		if s, ok := t.str(i, age); ok {
			if d, ok := parseAge(s); ok {
				r.created = now.Add(-d)
			}
		}
		if s, ok := t.str(i, node); ok && s != "<none>" {
			r.node = s
		}
		out = append(out, r)
	}
	return out
}

// missingColumns names the wanted columns a Table lacks.
func missingColumns(t table, want []string) []string {
	var out []string
	for _, c := range want {
		if t.col(c) < 0 {
			out = append(out, c)
		}
	}
	return out
}

// parseReady reads "a/b".
func parseReady(s string) (int, int, bool) {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		return 0, 0, false
	}
	x, err1 := strconv.Atoi(a)
	y, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return x, y, true
}

// recency is the last time a row changed visibly: its last restart, else
// its creation.
func recency(r podRow) time.Time {
	if r.lastRestart.After(r.created) {
		return r.lastRestart
	}
	return r.created
}

func derivePods(st store, led Ledger, scope Scope, now time.Time) PodsPanel {
	p := PodsPanel{
		Avail:   availOf(Pods, led),
		Cmd:     withScope(scope.Namespace, "get", "pods"),
		Scope:   scope.Namespace,
		Total:   len(st.pods),
		Missing: st.podsMissing,
	}
	failing := map[string]int{}
	perNode := map[string]int{}
	used := map[string][]bool{}
	for _, r := range rankedPods(st.pods) {
		c := classOf(r.status, r.ready, r.readyOf, r.readyOK)
		countPhase(&p.Phases, c)
		if severityOf(c) == SevFailing {
			failing[r.status]++
		}
		if c != ClassOK && c != ClassDone {
			p.NotOK++
		}
		if r.node != "" {
			perNode[r.node]++
		}
		row, keep := podRowOf(st, r, c, scope, now, used)
		switch {
		case !keep:
		case row.Sev > SevOK:
			p.Problems = append(p.Problems, row)
		case row.HasRestart && row.LastRestart < restartWindow && len(p.Restarted) < maxRestartedRows:
			row.Sev = SevInfo
			p.Restarted = append(p.Restarted, row)
		}
	}
	p.Failing = reasonCounts(failing)
	p.PerNode = nodeCounts(perNode)
	return p
}

// podRowOf builds the row for r, placing it with the cached lookup.
func podRowOf(st store, r podRow, c Class, scope Scope, now time.Time, used map[string][]bool) (PodRow, bool) {
	sev := severityOf(c)
	if sev == SevOK && r.restarts == 0 {
		return PodRow{}, false
	}
	row := PodRow{
		Key:        ObjectKey{Kind: "pod", Namespace: r.namespace, Name: r.name},
		Resolved:   r.namespace != "",
		Ready:      r.ready,
		ReadyOf:    r.readyOf,
		ReadyKnown: r.readyOK,
		Status:     r.status,
		Class:      c,
		Sev:        sev,
		Restarts:   r.restarts,
		HasRestart: !r.lastRestart.IsZero(),
		Node:       r.node,
	}
	row.LastRestart = since(now, r.lastRestart)
	row.Age = since(now, r.created)
	row.Starting = (c == ClassPending || c == ClassNotReady) && !r.created.IsZero() && now.Sub(r.created) < startGrace
	if e, ok := st.resolved[resolveKey{"pod", r.name}]; ok {
		if used[r.name] == nil {
			used[r.name] = make([]bool, len(e.objs))
		}
		if o, i, ok := matchPod(e.objs, r, c, used[r.name]); ok {
			used[r.name][i] = true
			row.Key.Namespace, row.Resolved = o.namespace, true
			row.Container, row.Init = o.container, o.init
		}
	}
	row.Cmd = podCmd(row, scope)
	return row, true
}

// podCmd describes a placed pod, or finds an unplaced one by name.
func podCmd(r PodRow, scope Scope) Cmd {
	if r.Resolved {
		return describe("pod", r.Key.Name, r.Key.Namespace)
	}
	return withScope(scope.Namespace, "get", "pods", "--field-selector", "metadata.name="+r.Key.Name)
}

func countPhase(p *PhaseCounts, c Class) {
	switch c {
	case ClassOK:
		p.Running++
	case ClassDone:
		p.Completed++
	case ClassPending, ClassNotReady:
		p.Waiting++
	case ClassCrash, ClassPull, ClassConfig:
		p.Failing++
	case ClassEvicted:
		p.Evicted++
	default:
		p.Other++
	}
}

func reasonCounts(m map[string]int) []ReasonCount {
	out := make([]ReasonCount, 0, len(m))
	for r, n := range m {
		out = append(out, ReasonCount{Reason: r, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func nodeCounts(m map[string]int) []NodeCount {
	out := make([]NodeCount, 0, len(m))
	for n, c := range m {
		out = append(out, NodeCount{Node: n, Pods: c})
	}
	slices.SortFunc(out, func(a, b NodeCount) int { return strings.Compare(a.Node, b.Node) })
	return out
}
