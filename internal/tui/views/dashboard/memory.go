package dashboard

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Memory is what the dashboard keeps between visits: the last snapshot and
// its ledger (which also parks denied endpoints), the history rings, "all
// clear since", the watch switch and where the cursor was. The root takes
// it from the outgoing dashboard (Model.Memory) into one field that
// survives going home and hands it to the next New, so coming back from
// :pods shows the data and the sparklines at once. :ctx drops it; a new
// namespace keeps only what is cluster-wide. The zero Memory is a first
// visit.
//
// Memory is a value: every change returns a new one, and the rings inside
// are never written in place.
type Memory struct {
	client *k8s.Client // the client it was learned through
	ns     string      // the namespace asked for, "" for all
	start  time.Time   // plan time zero, for the ledger
	fetch  FetchFunc
	srcErr error

	snap   dash.Snapshot
	loaded bool
	hist   history
	since  time.Time // the first all-clear snapshot of the current run
	focus  focus
	paused bool
	// pausedAt is when watch went off; changed is the wall time of the
	// last snapshot whose section hashes moved.
	pausedAt time.Time
	changed  time.Time
	// topByMemory is panel 7 sorted by memory (s), not CPU.
	topByMemory bool
}

// adopt returns the memory to start from for client c listing ns: a fresh
// one for another client (or none), this one rescoped for another
// namespace, else this one.
func (mem Memory) adopt(c *k8s.Client, ns string, now time.Time) Memory {
	switch {
	case c == nil:
		return Memory{}
	case mem.client != c:
		return freshMemory(c, ns, now)
	case mem.ns != ns:
		return mem.rescoped(ns)
	}
	return mem
}

func freshMemory(c *k8s.Client, ns string, now time.Time) Memory {
	mem := Memory{
		client: c,
		ns:     ns,
		start:  now,
		snap:   emptySnapshot(dash.Scope{Namespace: ns, Context: c.ContextNamespace}),
	}
	src, err := dash.NewSource(c)
	if err != nil {
		mem.srcErr = err
		return mem
	}
	mem.fetch = dash.NewFetcher(src).Fetch
	return mem
}

// emptySnapshot is a snapshot with nothing fetched yet whose panels already
// carry their commands, so the footer has one before the first answer.
// Rescope derives the panels; on an empty snapshot it drops nothing.
func emptySnapshot(scope dash.Scope) dash.Snapshot {
	return dash.NewSnapshot(scope).Rescope(scope)
}

// rescoped is the memory listing ns instead: namespaced data and the pods
// history go, the cursor leaves its row (that row is not in the new
// namespace), and the cluster-wide meters and node strips stay.
func (mem Memory) rescoped(ns string) Memory {
	mem.ns = ns
	mem.snap = mem.snap.Rescope(dash.Scope{Namespace: ns, Context: mem.client.ContextNamespace})
	mem.hist = mem.hist.rescoped()
	mem.since = time.Time{}
	mem.focus.row, mem.focus.index = "", 0
	return mem
}

// with folds in a new snapshot. The cursor is put back on its row by the
// model, which knows which rows the layout draws.
func (mem Memory) with(s dash.Snapshot) Memory {
	if !mem.loaded || s.Hash != mem.snap.Hash {
		mem.changed = s.At
	}
	mem.snap = s
	mem.hist = mem.hist.fold(s)
	mem.since = allClearSince(mem.since, s)
	if !mem.loaded && mem.focus.expanded == dash.PanelOverview {
		mem.focus.expanded = firstOpen(s)
	}
	mem.loaded = true
	return mem
}

// allClearSince keeps the time of the first all-clear snapshot while the
// verdict stays clear.
func allClearSince(since time.Time, s dash.Snapshot) time.Time {
	if !s.Verdict.AllClear {
		return time.Time{}
	}
	if since.IsZero() {
		return s.At
	}
	return since
}

// firstOpen is the compact accordion's first open panel: the first of
// pods, workloads, nodes and events with something failing, else pods.
func firstOpen(s dash.Snapshot) dash.Panel {
	failing := map[dash.Panel]bool{}
	for _, r := range s.Pods.Problems {
		failing[dash.PanelPods] = failing[dash.PanelPods] || r.Sev == dash.SevFailing
	}
	for _, r := range s.Workloads.Rows {
		failing[dash.PanelWorkloads] = failing[dash.PanelWorkloads] || r.Sev == dash.SevFailing
	}
	for _, r := range s.Nodes.Rows {
		failing[dash.PanelNodes] = failing[dash.PanelNodes] || r.Sev == dash.SevFailing
	}
	for _, p := range []dash.Panel{dash.PanelPods, dash.PanelWorkloads, dash.PanelNodes, dash.PanelEvents} {
		if failing[p] {
			return p
		}
	}
	return dash.PanelPods
}
