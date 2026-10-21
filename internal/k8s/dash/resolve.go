package dash

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
)

// A server-side Table with includeObject=None carries no namespace, and the
// pods Table has no per-container data. The few rows the panels flag get a
// typed lookup by name instead (fieldSelector=metadata.name=<n>), cached by
// name until the row changes, capped per refresh. Healthy rows never cost a
// request.

// resolveKey is one lookup: a kubectl kind word and an object name.
type resolveKey struct {
	kind string
	name string
}

// resolveEntry is a cached lookup. sig is the shape of the Table rows it was
// made for; a different shape means the cache is stale.
type resolveEntry struct {
	sig  string
	objs []resolvedObj
}

// resolvedObj is what a typed lookup adds to a Table row.
type resolvedObj struct {
	namespace string
	node      string
	class     Class
	restarts  int
	container string
	init      bool
	ready     int
	desired   int
}

// want is a lookup some panel would use, ranked: lower ranks go first.
type want struct {
	key  resolveKey
	sig  string
	rank int
	ns   string // namespace of the list the row came from; "" looks across all
}

// maxPodLookups bounds pod rows worth a lookup: the panel shows fewer.
const maxPodLookups = 20

// Lookup ranks: what the banner and the next line need first.
const (
	rankFailingPod = iota
	rankFailingWorkload
	rankComponent
	rankWaitingPod
	rankWaitingWorkload
	rankEvicted
	rankRestarted
	rankSuspended
)

// wanted lists the lookups the current rows call for, most urgent first.
func wanted(st store, k3s bool, now time.Time) []want {
	var out []want
	out = append(out, podWants(st, now)...)
	out = append(out, workloadWants(st, k3s)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].rank < out[j].rank })
	return out
}

// pending returns up to limit wanted lookups the cache cannot answer.
func pending(st store, ws []want, limit int) []want {
	var out []want
	for _, w := range ws {
		if len(out) >= limit {
			break
		}
		if e, ok := st.resolved[w.key]; ok && e.sig == w.sig {
			continue
		}
		out = append(out, w)
	}
	return out
}

// prune keeps only cache entries a current row still asks for.
func prune(cache map[resolveKey]resolveEntry, ws []want) map[resolveKey]resolveEntry {
	keep := make(map[resolveKey]resolveEntry, len(ws))
	for _, w := range ws {
		if e, ok := cache[w.key]; ok {
			keep[w.key] = e
		}
	}
	return keep
}

func podWants(st store, now time.Time) []want {
	shapes := map[string][]string{}
	spaces := map[string]string{}
	ranks := map[string]int{}
	restarted := 0
	for _, r := range rankedPods(st.pods) {
		c := classOf(r.status, r.ready, r.readyOf, r.readyOK)
		// A namespaced list already names the namespace; only a crash still
		// needs its container.
		if !validName(r.name) || (r.namespace != "" && c != ClassCrash) {
			continue
		}
		rank, ok := podRank(c, r, now, &restarted)
		if !ok {
			continue
		}
		if _, seen := ranks[r.name]; !seen {
			if len(ranks) >= maxPodLookups {
				continue
			}
			ranks[r.name] = rank
			spaces[r.name] = r.namespace
		}
		shapes[r.name] = append(shapes[r.name], r.node+"|"+strconv.Itoa(int(c)))
	}
	out := make([]want, 0, len(ranks))
	for name, rank := range ranks {
		sig := shapes[name]
		slices.Sort(sig)
		out = append(out, want{key: resolveKey{"pod", name}, sig: strings.Join(sig, ","), rank: rank, ns: spaces[name]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].key.name < out[j].key.name
	})
	return out
}

// podRank orders lookups: failing pods, then waiting, evicted, and up to
// three recently restarted ones.
func podRank(c Class, r podRow, now time.Time, restarted *int) (int, bool) {
	switch severityOf(c) {
	case SevFailing:
		return rankFailingPod, true
	case SevWaiting:
		return rankWaitingPod, true
	case SevMinor:
		return rankEvicted, true
	}
	recent := !r.lastRestart.IsZero() && now.Sub(r.lastRestart) < restartWindow
	if c == ClassOK && r.restarts > 0 && recent && *restarted < maxRestartedRows {
		*restarted++
		return rankRestarted, true
	}
	return 0, false
}

// rankedPods orders rows the way the panel does, so the cap keeps the rows
// that will be drawn.
func rankedPods(rows []podRow) []podRow {
	out := slices.Clone(rows)
	sort.SliceStable(out, func(i, j int) bool {
		si := severityOf(classOf(out[i].status, out[i].ready, out[i].readyOf, out[i].readyOK))
		sj := severityOf(classOf(out[j].status, out[j].ready, out[j].readyOf, out[j].readyOK))
		if si != sj {
			return si > sj
		}
		return recency(out[i]).After(recency(out[j]))
	})
	return out
}

func workloadWants(st store, k3s bool) []want {
	var out []want
	add := func(kind string, rows []workloadRow, keep func(workloadRow) (int, bool)) {
		counts := map[string]int{}
		for _, r := range rows {
			counts[r.name]++
		}
		for _, r := range rows {
			rank, ok := keep(r)
			if !ok || r.namespace != "" || !validName(r.name) || counts[r.name] < 0 {
				continue
			}
			out = append(out, want{key: resolveKey{kind, r.name}, sig: strconv.Itoa(counts[r.name]), rank: rank})
			counts[r.name] = -1 // one lookup per name
		}
	}
	notReady := func(r workloadRow) (int, bool) {
		sev := workloadSeverity(r.ready, r.desired)
		switch {
		case sev == SevFailing:
			return rankFailingWorkload, true
		case sev == SevWaiting:
			return rankWaitingWorkload, true
		case k3s && isComponent(r.name):
			return rankComponent, true
		default:
			return 0, false
		}
	}
	add("deployment", st.deploys, notReady)
	add("statefulset", st.sts, notReady)
	add("daemonset", st.ds, notReady)
	cron := make([]workloadRow, 0, len(st.cronjobs))
	for _, c := range st.cronjobs {
		if c.suspend {
			cron = append(cron, workloadRow{name: c.name, namespace: c.namespace})
		}
	}
	add("cronjob", cron, func(workloadRow) (int, bool) { return rankSuspended, true })
	svc := make([]workloadRow, 0, len(st.services))
	for _, s := range st.services {
		if s.pending {
			svc = append(svc, workloadRow{name: s.name, namespace: s.namespace})
		}
	}
	add("service", svc, func(workloadRow) (int, bool) { return rankWaitingWorkload, true })
	return out
}

// validName keeps Table text that is not an object name out of the field
// selector, where a comma would add a selector term.
func validName(name string) bool {
	return len(validation.IsDNS1123Subdomain(name)) == 0
}

// matchPod picks the looked-up pod behind a Table row: the only one, else
// the one on the same node, of the same class, with the same restarts.
// used marks objects already given to an earlier row with the same name.
func matchPod(objs []resolvedObj, r podRow, c Class, used []bool) (resolvedObj, int, bool) {
	tests := []func(resolvedObj) bool{
		func(o resolvedObj) bool { return o.node == r.node },
		func(o resolvedObj) bool { return o.class == c },
		func(o resolvedObj) bool { return o.restarts == r.restarts },
	}
	cands := make([]int, 0, len(objs))
	for i := range objs {
		if !used[i] {
			cands = append(cands, i)
		}
	}
	for _, test := range tests {
		if len(cands) <= 1 {
			break
		}
		narrowed := slices.DeleteFunc(slices.Clone(cands), func(i int) bool { return !test(objs[i]) })
		if len(narrowed) > 0 {
			cands = narrowed
		}
	}
	if len(cands) == 0 {
		return resolvedObj{}, -1, false
	}
	return objs[cands[0]], cands[0], true
}

// matchWorkload picks the looked-up object behind a workload row: the only
// one, else the first unused with the same ready and desired counts.
func matchWorkload(objs []resolvedObj, r workloadRow, used []bool) (resolvedObj, int, bool) {
	first := -1
	for i, o := range objs {
		if used[i] {
			continue
		}
		if first < 0 {
			first = i
		}
		if o.ready == r.ready && o.desired == r.desired {
			return o, i, true
		}
	}
	if first < 0 {
		return resolvedObj{}, -1, false
	}
	return objs[first], first, true
}
