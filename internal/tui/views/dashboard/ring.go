package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// The change ring is "≈ seen by k4s": state changes k4s noticed between
// two polls, newest first, kept in memory for this session only. Nothing
// is fetched for it; it is a diff of two snapshots, and only of sources
// that answered both times, so a list going stale or forbidden never
// reads as everything cleared.

// ringLimit is how many changes the ring keeps.
const ringLimit = 20

// change is one state change between two polls.
type change struct {
	at       time.Time
	sev      dash.Severity
	obj      dash.ObjectKey
	from, to string
	// cmd is the data layer's command for the row the change was seen on:
	// describe once the object's namespace is known (resolved), a lookup
	// by name before. Later polls refresh it (see refreshed); it is zero
	// once the object is gone.
	cmd      dash.Cmd
	resolved bool
	gone     bool
}

// object is the change's object as the ring prints it: "pod shop/api".
func (c change) object() string {
	name := c.obj.Name
	if c.obj.Namespace != "" {
		name = c.obj.Namespace + "/" + name
	}
	return shortKind(c.obj.Kind) + " " + clean(name)
}

// refreshed is ring as of snapshot s, in a new slice: see after.
func refreshed(ring []change, s dash.Snapshot) []change {
	out := make([]change, len(ring))
	for i, c := range ring {
		out[i] = c.after(s)
	}
	return out
}

// after is c as of snapshot s: an object the lists no longer hold is gone
// and has no command left; one found by name until now takes its row's
// key and command once the data layer has placed it in its namespace.
func (c change) after(s dash.Snapshot) change {
	if c.gone {
		return c
	}
	if exists, known := s.Exists(c.obj); known && !exists {
		c.gone, c.cmd = true, dash.Cmd{}
		return c
	}
	if c.resolved {
		return c
	}
	if key, cmd, ok := placedRow(s, c.obj); ok {
		c.obj, c.cmd, c.resolved = key, cmd, true
	}
	return c
}

// placedRow finds the one pod or workload row of s that is k placed in its
// namespace; two rows of that name leave it unplaced.
func placedRow(s dash.Snapshot, k dash.ObjectKey) (dash.ObjectKey, dash.Cmd, bool) {
	var key dash.ObjectKey
	var cmd dash.Cmd
	n := 0
	match := func(rk dash.ObjectKey, resolved bool, rc dash.Cmd) {
		if resolved && rk.Kind == k.Kind && rk.Name == k.Name {
			key, cmd, n = rk, rc, n+1
		}
	}
	for _, r := range s.Pods.Problems {
		match(r.Key, r.Resolved, r.Cmd)
	}
	for _, r := range s.Workloads.Rows {
		match(r.Key, r.Resolved, r.Cmd)
	}
	return key, cmd, n == 1
}

// changesBetween lists what changed from prev to next, most severe first.
func changesBetween(prev, next dash.Snapshot) []change {
	out := slices.Concat(podChanges(prev, next), workloadChanges(prev, next), nodeChanges(prev, next))
	slices.SortStableFunc(out, func(a, b change) int { return int(b.sev) - int(a.sev) })
	return out
}

// withRing returns ring with changes in front, at most ringLimit, in a new
// slice.
func withRing(ring, changes []change) []change {
	out := slices.Concat(changes, ring)
	return out[:min(len(out), ringLimit)]
}

func answered(a dash.Avail) bool { return a.State == dash.StateOK }

// stated is one row's state for the diff, with the row's command and
// whether its namespace is known.
type stated struct {
	key      dash.ObjectKey
	state    string
	sev      dash.Severity
	cmd      dash.Cmd
	resolved bool
}

// rowChanges diffs two keyed lists of states: a row that appears goes
// from okWord to its state, one that changes from one to the other, one
// that leaves to goneWord. Each change carries the command of the row it
// was seen on.
func rowChanges(at time.Time, prev, next []stated, okWord, goneWord func(dash.ObjectKey) string) []change {
	was := make(map[string]stated, len(prev))
	for _, s := range prev {
		was[rowName(s.key)] = s
	}
	now := make(map[string]bool, len(next))
	var out []change
	seen := func(s stated, sev dash.Severity, from, to string) change {
		return change{at: at, sev: sev, obj: s.key, from: from, to: to, cmd: s.cmd, resolved: s.resolved}
	}
	for _, s := range next {
		now[rowName(s.key)] = true
		p, ok := was[rowName(s.key)]
		switch {
		case !ok:
			out = append(out, seen(s, s.sev, okWord(s.key), s.state))
		case p.state != s.state:
			out = append(out, seen(s, s.sev, p.state, s.state))
		}
	}
	for _, p := range prev {
		if !now[rowName(p.key)] {
			out = append(out, seen(p, dash.SevOK, p.state, goneWord(p.key)))
		}
	}
	return out
}

func podChanges(prev, next dash.Snapshot) []change {
	if !answered(prev.Pods.Avail) || !answered(next.Pods.Avail) {
		return nil
	}
	states := func(rows []dash.PodRow) []stated {
		out := make([]stated, 0, len(rows))
		for _, r := range rows {
			out = append(out, stated{key: r.Key, state: clean(r.Status), sev: r.Sev, cmd: r.Cmd, resolved: r.Resolved})
		}
		return out
	}
	return rowChanges(next.At, states(prev.Pods.Problems), states(next.Pods.Problems), word("ok"), word("cleared"))
}

func workloadChanges(prev, next dash.Snapshot) []change {
	if !answered(prev.Workloads.Deploy.Avail) || !answered(next.Workloads.Deploy.Avail) {
		return nil
	}
	states := func(rows []dash.WorkloadRow) []stated {
		out := make([]stated, 0, len(rows))
		for _, r := range rows {
			out = append(out, stated{key: r.Key, state: workloadState(r), sev: r.Sev, cmd: r.Cmd, resolved: r.Resolved})
		}
		return out
	}
	return rowChanges(next.At, states(prev.Workloads.Rows), states(next.Workloads.Rows), workloadOK, workloadOK)
}

// workloadState is a workload row's state as the ring prints it.
func workloadState(r dash.WorkloadRow) string {
	switch {
	case r.Failed:
		return "failed"
	case r.Suspended:
		return "suspended"
	}
	return fmt.Sprintf("%d/%d", r.Ready, r.Desired)
}

// workloadOK is the state of a workload no panel flags.
func workloadOK(k dash.ObjectKey) string {
	switch k.Kind {
	case "job":
		return "ok"
	case "cronjob":
		return "active"
	}
	return "ready"
}

func nodeChanges(prev, next dash.Snapshot) []change {
	if !answered(prev.Nodes.Avail) || !answered(next.Nodes.Avail) {
		return nil
	}
	states := func(rows []dash.NodeRow) []stated {
		out := make([]stated, 0, len(rows))
		for _, r := range rows {
			words := append([]string{clean(r.Condition)}, r.Pressures...)
			if r.Unschedulable {
				words = append(words, "SchedulingDisabled")
			}
			// A node has no namespace to find: it is placed as listed.
			out = append(out, stated{key: r.Key, state: strings.Join(words, ","), sev: r.Sev, cmd: r.Cmd, resolved: true})
		}
		return out
	}
	return rowChanges(next.At, states(prev.Nodes.Rows), states(next.Nodes.Rows), word("absent"), word("gone"))
}

// word is a state word that does not depend on the object.
func word(w string) func(dash.ObjectKey) string {
	return func(dash.ObjectKey) string { return w }
}
