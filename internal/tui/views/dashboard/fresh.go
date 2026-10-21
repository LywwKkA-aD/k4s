package dashboard

import (
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// freshFor is how long a problem row that appeared carries its "new" tag:
// until the first poll at least this long after it appeared (§6).
const freshFor = 30 * time.Second

// rowName names a problem row by kind and name. The namespace is left
// out: a row first listed without one gets it from a lookup a poll
// later, and must not read as new again.
func rowName(k dash.ObjectKey) string {
	return k.Kind + "/" + k.Name
}

// problemNames are the pod and workload rows that need a look in s, when
// their lists answered; ok is false when either did not.
func problemNames(s dash.Snapshot) (map[string]bool, bool) {
	if !readable(s.Pods.Avail) || !readable(s.Workloads.Deploy.Avail) {
		return nil, false
	}
	out := map[string]bool{}
	for _, r := range s.Pods.Problems {
		out[rowName(r.Key)] = true
	}
	for _, r := range s.Workloads.Rows {
		out[rowName(r.Key)] = true
	}
	return out, true
}

// freshRows carries the new tags from prev into the snapshot after old:
// rows that were not in old get one stamped now, tags freshFor old go.
// The first snapshot tags nothing, and neither does one after a list that
// did not answer: everything would look new.
func freshRows(prev map[string]time.Time, old, s dash.Snapshot, loaded bool) map[string]time.Time {
	now, okNow := problemNames(s)
	was, okWas := problemNames(old)
	out := map[string]time.Time{}
	if !loaded || !okNow || !okWas {
		return out
	}
	for name := range now {
		since, tagged := prev[name]
		switch {
		case tagged && s.At.Sub(since) < freshFor:
			out[name] = since
		case !tagged && !was[name]:
			out[name] = s.At
		}
	}
	return out
}
