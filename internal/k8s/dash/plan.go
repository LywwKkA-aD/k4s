package dash

import "time"

// Focus is what the view knows about its own visibility.
type Focus struct {
	Active  bool          // the dashboard is the current view
	Blurred bool          // the terminal reported focus loss
	Idle    time.Duration // time since the last key press
	TopPods bool          // panel 7 is drawn or zoomed
}

// Caps are cluster facts learned from earlier snapshots that change which
// endpoints exist at all.
type Caps struct {
	Metrics    bool // metrics.k8s.io/v1beta1 is in discovery and answering
	K3s        bool // the server version carries +k3s
	Etcd       bool // a node carries node-role.kubernetes.io/etcd
	LBFallback bool // the server rejects the spec.type field selector (before 1.31)
	// VersionGiven is set when the caller asks /version and hands the
	// answer over (Snapshot.GiveVersion): the plan never asks it, not on r.
	VersionGiven bool
}

// PlanInput is everything Plan needs. It carries no clock: Now is the
// caller's elapsed time since the dashboard session began.
type PlanInput struct {
	Now      time.Duration
	Watch    bool // auto refresh on
	Force    bool // r: every tier, consistent reads, parked endpoints re-probed
	Focus    Focus
	Scope    Scope
	Ledger   Ledger
	Caps     Caps
	Throttle int // consecutive refreshes that met a 429
	Failures int // consecutive refreshes where every request timed out
}

// Request is one due endpoint with the list options to send.
type Request struct {
	Endpoint   Endpoint
	Namespace  string // "" is cluster-wide or all namespaces
	Consistent bool   // RV="" instead of RV="0"
	Limit      int64
	Fallback   bool // LoadBalancers: list the services Table and filter client-side
}

// Plan is the pure decision for one refresh: what to request now and when
// the next tick is due.
type Plan struct {
	Now        time.Duration
	Force      bool
	Scope      Scope
	Due        []Request
	Arm        bool          // schedule a next tick at all
	Next       time.Duration // delay of the next tick; 0 when Arm is false
	Cadence    time.Duration // fast-tier interval in effect, for the "watch 5s" hint
	Slowed     bool          // stretched by payload size, wall time, 429 or timeouts
	ResolveCap int           // most typed lookups of problem objects this refresh
}

// never is the wait of a once-per-context endpoint already fetched.
const never = time.Duration(1<<63 - 1)

// resolveCap bounds the typed lookups that give Table rows their namespace
// and crashing container.
const resolveCap = 10

// MakePlan returns the due set and the next interval for in. It reads no
// clock and starts nothing.
func MakePlan(in PlanInput) Plan {
	p := Plan{
		Now:        in.Now,
		Force:      in.Force,
		Scope:      in.Scope,
		Cadence:    fastInterval(in),
		ResolveCap: resolveCap,
	}
	p.Slowed = p.Cadence > fastStretch(in.Focus)
	next := time.Duration(-1)
	for e := range numEndpoints {
		if !relevant(e, in) {
			continue
		}
		wait := remaining(e, in)
		if wait <= 0 || in.Force {
			p.Due = append(p.Due, request(e, in))
			wait = intervalOf(e, in)
		}
		if wait > 0 && wait < never && (next < 0 || wait < next) {
			next = wait
		}
	}
	p.Arm = in.Watch && in.Focus.Active
	if !p.Arm {
		return p
	}
	backoff := backoffFor(in.Throttle, in.Failures)
	p.Slowed = p.Slowed || backoff > 0
	p.Next = max(next, base, backoff)
	return p
}

// relevant reports whether an endpoint exists for this cluster and view.
func relevant(e Endpoint, in PlanInput) bool {
	discovered := in.Caps.Metrics || !in.Ledger[MetricsDiscovery].Fetched
	switch e {
	case NodeMetrics:
		return discovered
	case PodMetrics:
		return discovered && in.Focus.TopPods
	case HelmCharts:
		return in.Caps.K3s
	case EtcdSnapshots:
		return in.Caps.K3s && in.Caps.Etcd
	case ServiceCount:
		return !in.Caps.LBFallback
	case Version:
		return !in.Caps.VersionGiven
	default:
		return true
	}
}

// remaining is how long until e is due; zero or less means due now. A tick
// a little early still counts, so the tiers stay aligned on 5 s ticks.
func remaining(e Endpoint, in PlanInput) time.Duration {
	u := in.Ledger[e]
	if !u.Fetched {
		return 0
	}
	if parked(e, in) {
		return u.At + parkFor - in.Now
	}
	iv := intervalOf(e, in)
	if iv < 0 {
		if onceDone(u.State) {
			return never
		}
		iv = fastInterval(in) // a transient failure must not pin the answer to empty
	}
	left := u.At + iv - in.Now
	if left <= base/2 {
		return 0
	}
	return left
}

// onceDone reports whether a once-per-context endpoint has its final answer.
// A timeout, 429, 5xx or expired credential is retried like a fast endpoint.
func onceDone(s State) bool {
	switch s {
	case StateOK, StateForbidden, StateAbsent, StateUnsupported:
		return true
	default:
		return false
	}
}

// parked reports a 403 or 404 that is waiting out parkFor. The key is the
// endpoint and namespace, so a scope change re-probes at once.
func parked(e Endpoint, in PlanInput) bool {
	u := in.Ledger[e]
	if in.Force || (u.State != StateForbidden && u.State != StateAbsent) {
		return false
	}
	if e == MetricsDiscovery || e == NodeMetrics || e == PodMetrics {
		return false // discovery re-checks on its own cadence
	}
	if u.Namespace != namespaceFor(e, in.Scope) {
		return false
	}
	return in.Now-u.At < parkFor
}

func namespaceFor(e Endpoint, s Scope) string {
	if e.Namespaced() {
		return s.Namespace
	}
	return ""
}

func request(e Endpoint, in PlanInput) Request {
	r := Request{
		Endpoint:   e,
		Namespace:  namespaceFor(e, in.Scope),
		Consistent: in.Force,
	}
	switch e {
	case ServiceCount:
		r.Limit, r.Consistent = 1, true // RV=0 ignores limit and returns the whole list
	case Events:
		if in.Ledger[Events].Rows > eventCap {
			r.Limit, r.Consistent = eventCap, true
		}
	case LoadBalancers:
		r.Fallback = in.Caps.LBFallback
	}
	return r
}
