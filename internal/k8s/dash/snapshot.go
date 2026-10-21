package dash

import (
	"fmt"
	"hash/fnv"
	"time"
)

// Snapshot is one refresh of the whole dashboard: every panel as data,
// the verdict and next suggestions, and the ledger the next Plan reads.
// Endpoints that were not due keep their previous data. A Snapshot is a
// value; Fetch returns a new one and never changes the one it was given.
type Snapshot struct {
	Now      time.Duration // plan time of this refresh
	At       time.Time     // wall time of this refresh, from the fetcher's clock
	Scope    Scope         // the scope actually listed, after any -A fallback
	Ledger   Ledger
	Caps     Caps
	Throttle int // consecutive refreshes that met a 429
	Failures int // consecutive refreshes where every request timed out
	Requests int // requests this refresh made
	Bytes    int // body bytes this refresh decoded

	Cluster   ClusterPanel
	Nodes     NodesPanel
	Pods      PodsPanel
	Workloads WorkloadsPanel
	Events    EventsPanel
	K3s       K3sPanel
	TopPods   TopPodsPanel
	Verdict   Verdict
	Next      []Suggestion // up to 5, empty when there is nothing to fix
	Daily     Suggestion   // the command of the day, for the all-clear banner
	Hash      Hashes

	st   store
	host string
}

// Hashes are FNV-64 sums of each section; a panel re-renders only when
// its sum changes. Relative times inside are coarsened, so a poll that
// only moves the clock changes nothing.
type Hashes struct {
	Cluster, Nodes, Pods, Workloads, Events, K3s, TopPods, Verdict, Next uint64
}

// NewSnapshot is the empty snapshot a dashboard starts from. scope.Context
// is the kubeconfig context's namespace, used when -A is denied.
func NewSnapshot(scope Scope) Snapshot { return Snapshot{Scope: scope} }

// Rescope returns s listing scope instead, as after :ns. Namespaced data
// and its ledger entries are dropped, so those endpoints are due at once
// and parked denials are re-probed; cluster-wide data stays.
func (s Snapshot) Rescope(scope Scope) Snapshot {
	out := s
	out.Scope = scope
	out.st.resolved = map[resolveKey]resolveEntry{}
	for e := range numEndpoints {
		if e.Namespaced() {
			out.Ledger[e] = Usage{}
			clearData(&out.st, e)
		}
	}
	return derive(out)
}

// PlanInput is the input for the plan after s. now is the caller's
// elapsed time; watch, force and focus come from the view.
func (s Snapshot) PlanInput(now time.Duration, watch, force bool, focus Focus) PlanInput {
	return PlanInput{
		Now: now, Watch: watch, Force: force, Focus: focus, Scope: s.Scope,
		Ledger: s.Ledger, Caps: s.Caps, Throttle: s.Throttle, Failures: s.Failures,
	}
}

// derive rebuilds every panel from the stored payloads. It is pure: now
// is the snapshot's wall time.
func derive(s Snapshot) Snapshot {
	st, led, scope, now := s.st, s.Ledger, s.Scope, s.At
	s.Caps = capsOf(st)
	s.Cluster = deriveCluster(st, led, scope, s.host, now)
	s.Nodes = deriveNodes(st, led, now)
	s.Pods = derivePods(st, led, scope, now)
	s.Workloads = deriveWorkloads(st, led, scope, now)
	s.Events = deriveEvents(st, led, scope, flaggedOf(s), now)
	s.K3s = deriveK3s(st, led, scope, s.Cluster.Version.Distro, now)
	s.TopPods = deriveTopPods(st, led, scope)
	s.Cluster.Network.PendingObjects = pendingLoadBalancers(st)
	s.Verdict = verdictOf(s)
	s.Next = suggest(s)
	s.Daily = CommandOfTheDay(now.YearDay(), envOf(s))
	s.Hash = Hashes{
		Cluster: sum(s.Cluster), Nodes: sum(s.Nodes), Pods: sum(s.Pods), Workloads: sum(s.Workloads),
		Events: sum(s.Events), K3s: sum(s.K3s), TopPods: sum(s.TopPods), Verdict: sum(s.Verdict), Next: sum(s.Next),
	}
	return s
}

func sum(v any) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%+v", v)
	return h.Sum64()
}

func capsOf(st store) Caps {
	c := Caps{Metrics: st.metricsOK, K3s: distroOf(st.version.git) == "k3s", LBFallback: st.lbFallback}
	for _, n := range st.nodes {
		c.Etcd = c.Etcd || n.etcd
	}
	return c
}

// availOf turns an endpoint's ledger entry into a panel Avail.
func availOf(e Endpoint, led Ledger) Avail {
	u := led[e]
	a := Avail{State: u.State}
	switch u.State {
	case StateForbidden:
		a.CanI = canIOf(e, u.Namespace)
	case StateStale:
		a.Stale = Coarsen(u.At - u.StaleSince)
	}
	return a
}

// canIOf is the probe for exactly the request that was denied, in the
// namespace it was made in.
func canIOf(e Endpoint, ns string) Cmd {
	switch e {
	case Readyz:
		return kubectl("auth", "can-i", "get", "/readyz")
	case Version:
		return kubectl("auth", "can-i", "get", "/version")
	case Whoami:
		return kubectl("auth", "can-i", "create", "selfsubjectreviews")
	}
	return canIList(e.resource(), ns, e.Namespaced())
}

// flaggedOf collects what the pods, workloads and nodes panels already
// show, and what exists, for the warning rule.
func flaggedOf(s Snapshot) flagged {
	f := flagged{objects: map[ObjectKey]bool{}, names: map[string]bool{}}
	for _, r := range s.Pods.Problems {
		mark(f, r.Key, r.Resolved)
	}
	for _, r := range s.Workloads.Rows {
		mark(f, r.Key, r.Resolved)
		if r.Key.Kind == "deployment" && r.Sev > SevMinor {
			f.deploys = append(f.deploys, r.Key.Name)
		}
	}
	for _, r := range s.Nodes.Rows {
		if r.Sev >= SevWaiting {
			f.objects[r.Key] = true
		}
	}
	return existing(f, s)
}

// existing fills in what the lists say exists; a list that did not answer
// leaves its map nil, which the warning rule reads as "assume it exists".
func existing(f flagged, s Snapshot) flagged {
	if s.Ledger[Pods].State == StateOK {
		f.pods = make(map[string]podRow, len(s.st.pods))
		for _, r := range s.st.pods {
			f.pods[r.name] = r
		}
	}
	if s.Ledger[Nodes].State == StateOK {
		f.nodes = make(map[string]bool, len(s.st.nodes))
		for _, n := range s.st.nodes {
			f.nodes[n.name] = true
		}
	}
	if s.Ledger[Deployments].State == StateOK && s.Ledger[StatefulSets].State == StateOK && s.Ledger[DaemonSets].State == StateOK {
		f.workloads = map[string]bool{}
		for _, kr := range workloadKinds(s.st) {
			for _, r := range kr.rows {
				f.workloads[kr.kind+"/"+r.name] = true
			}
		}
	}
	return f
}

func mark(f flagged, k ObjectKey, resolved bool) {
	if resolved {
		f.objects[k] = true
		return
	}
	f.names[k.Kind+"/"+k.Name] = true
}

// pendingLoadBalancers are the placed LoadBalancers still <pending>.
func pendingLoadBalancers(st store) []ObjectKey {
	var out []ObjectKey
	if !st.lbFallback {
		for _, s := range st.lbs {
			if s.pending {
				out = append(out, ObjectKey{Kind: "service", Namespace: s.namespace, Name: s.name})
			}
		}
		return out
	}
	used := map[string][]bool{}
	for _, s := range st.services {
		if !s.pending {
			continue
		}
		if key, ok := placeWorkload(st, "service", workloadRow{name: s.name, namespace: s.namespace}, used); ok {
			out = append(out, key)
		}
	}
	return out
}
