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
}

// object is the change's object as the ring prints it: "pod shop/api".
func (c change) object() string {
	name := c.obj.Name
	if c.obj.Namespace != "" {
		name = c.obj.Namespace + "/" + name
	}
	return shortKind(c.obj.Kind) + " " + clean(name)
}

// cmd describes the object.
func (c change) cmd() dash.Cmd {
	args := []string{"describe", c.obj.Kind, c.obj.Name}
	if c.obj.Namespace != "" {
		args = append(args, "-n", c.obj.Namespace)
	}
	return dash.Cmd{Args: args}
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

// stated is one row's state for the diff.
type stated struct {
	key   dash.ObjectKey
	state string
	sev   dash.Severity
}

// rowChanges diffs two keyed lists of states: a row that appears goes
// from okWord to its state, one that changes from one to the other, one
// that leaves to goneWord.
func rowChanges(at time.Time, prev, next []stated, okWord, goneWord func(dash.ObjectKey) string) []change {
	was := make(map[string]stated, len(prev))
	for _, s := range prev {
		was[rowName(s.key)] = s
	}
	now := make(map[string]bool, len(next))
	var out []change
	for _, s := range next {
		now[rowName(s.key)] = true
		p, ok := was[rowName(s.key)]
		switch {
		case !ok:
			out = append(out, change{at: at, sev: s.sev, obj: s.key, from: okWord(s.key), to: s.state})
		case p.state != s.state:
			out = append(out, change{at: at, sev: s.sev, obj: s.key, from: p.state, to: s.state})
		}
	}
	for _, p := range prev {
		if !now[rowName(p.key)] {
			out = append(out, change{at: at, sev: dash.SevOK, obj: p.key, from: p.state, to: goneWord(p.key)})
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
			out = append(out, stated{key: r.Key, state: clean(r.Status), sev: r.Sev})
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
			out = append(out, stated{key: r.Key, state: workloadState(r), sev: r.Sev})
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
			out = append(out, stated{key: r.Key, state: strings.Join(words, ","), sev: r.Sev})
		}
		return out
	}
	return rowChanges(next.At, states(prev.Nodes.Rows), states(next.Nodes.Rows), word("absent"), word("gone"))
}

// word is a state word that does not depend on the object.
func word(w string) func(dash.ObjectKey) string {
	return func(dash.ObjectKey) string { return w }
}
