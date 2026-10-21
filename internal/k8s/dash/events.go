package dash

import (
	"slices"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	recentEvent  = 10 * time.Minute
	eventHour    = time.Hour
	eventBucket  = 2 * time.Minute
	maxEventRows = 100
)

// benignReasons are kubelet warnings that recur on healthy nodes and say
// nothing about a workload, such as more than three nameservers on the host.
var benignReasons = []string{"DNSConfigForming"}

// probeReasons are probe failures; on a pod that is Running and ready now
// the probe has recovered.
var probeReasons = []string{"Unhealthy", "ProbeWarning"}

// eventsFrom keeps the fields the panel reads from a type=Warning list. The
// selector is re-applied here: a fake clientset ignores it.
func eventsFrom(items []corev1.Event) []eventInfo {
	out := make([]eventInfo, 0, len(items))
	for _, e := range items {
		if e.Type != corev1.EventTypeWarning {
			continue
		}
		out = append(out, eventInfo{
			kind:      e.InvolvedObject.Kind,
			namespace: e.InvolvedObject.Namespace,
			name:      e.InvolvedObject.Name,
			fieldPath: e.InvolvedObject.FieldPath,
			reason:    e.Reason,
			message:   e.Message,
			count:     max(e.Count, 1),
			last:      lastSeen(e),
		})
	}
	return out
}

// lastSeen is series.lastObservedTime, then lastTimestamp, then eventTime:
// the order kubectl events sorts by.
func lastSeen(e corev1.Event) time.Time {
	switch {
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	default:
		return e.EventTime.Time
	}
}

// containerOf reads spec.containers{name} or spec.initContainers{name}.
func containerOf(fieldPath string) string {
	for _, prefix := range []string{"spec.containers{", "spec.initContainers{", "spec.ephemeralContainers{"} {
		if rest, ok := strings.CutPrefix(fieldPath, prefix); ok {
			name, _, _ := strings.Cut(rest, "}")
			return name
		}
	}
	return ""
}

// kindWord turns an involvedObject kind into the kubectl resource word.
func kindWord(kind string) string { return strings.ToLower(kind) }

// flagged is what other panels already show as a problem, so its warnings
// are not counted twice, and what exists, so stale warnings are dropped.
type flagged struct {
	objects   map[ObjectKey]bool   // problems placed in a namespace, and nodes
	names     map[string]bool      // "kind/name" of problems not yet placed
	deploys   []string             // not-ready deployments: their ReplicaSets' warnings are theirs
	pods      map[ObjectKey]podRow // pods the list placed in a namespace; nil when the pods list is unavailable
	podNames  map[string]podRow    // pods the list did not place, by name
	nodes     map[string]bool      // nil when the nodes list is unavailable
	workloads map[ObjectKey]bool   // placed workloads; nil when the lists are unavailable
	// workloadNames are the unplaced ones, "kind/name".
	workloadNames map[string]bool
}

// has reports whether another panel already flags the object.
func (f flagged) has(key ObjectKey) bool {
	return f.objects[key] || f.names[key.Kind+"/"+key.Name]
}

func deriveEvents(st store, led Ledger, scope Scope, f flagged, now time.Time) EventsPanel {
	p := EventsPanel{
		Avail:     availOf(Events, led),
		Cmd:       withScope(scope.Namespace, "events", "--types=Warning"),
		Total:     max(st.eventsTotal, len(st.events)),
		Truncated: st.eventsCapped,
	}
	reasons, objects := map[string]bool{}, map[ObjectKey]bool{}
	sorted := append([]eventInfo(nil), st.events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].last.After(sorted[j].last) })
	for _, e := range sorted {
		key := ObjectKey{Kind: kindWord(e.kind), Namespace: e.namespace, Name: e.name}
		reasons[e.reason], objects[key] = true, true
		age := now.Sub(e.last)
		if age >= 0 && age < eventHour {
			p.LastHour[len(p.LastHour)-1-int(age/eventBucket)]++
		}
		counted := counts(e, key, age, f)
		if counted {
			p.Counted++
		}
		if len(p.Rows) < maxEventRows {
			p.Rows = append(p.Rows, EventRow{
				Object: key, Container: containerOf(e.fieldPath), Reason: e.reason, Message: e.message,
				Count: e.count, Ago: Coarsen(age), Counted: counted, Flagged: f.has(key), Cmd: describe(key.Kind, key.Name, key.Namespace),
			})
		}
	}
	p.Reasons, p.Objects = len(reasons), len(objects)
	return p
}

// counts is the critique's warning rule: a warning moves the verdict only
// when it is recent, not routine, about an object that still exists and
// that no other panel already flags, and not a probe that has recovered.
func counts(e eventInfo, key ObjectKey, age time.Duration, f flagged) bool {
	if age >= recentEvent || slices.Contains(benignReasons, e.reason) {
		return false
	}
	if f.has(key) {
		return false
	}
	short := key.Kind + "/" + key.Name
	switch key.Kind {
	case "pod":
		if f.pods == nil {
			return true
		}
		r, ok := f.pods[key]
		if !ok {
			r, ok = f.podNames[key.Name]
		}
		if !ok {
			return false
		}
		recovered := classOf(r.status, r.ready, r.readyOf, r.readyOK) == ClassOK
		return !recovered || !slices.Contains(probeReasons, e.reason)
	case "node":
		return f.nodes == nil || f.nodes[key.Name]
	case "replicaset":
		for _, d := range f.deploys {
			if strings.HasPrefix(key.Name, d+"-") {
				return false
			}
		}
	case "deployment", "statefulset", "daemonset":
		return f.workloads == nil || f.workloads[key] || f.workloadNames[short]
	}
	return true
}
