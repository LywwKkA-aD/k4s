package dash

import (
	"context"
	"sync"
	"time"
)

const (
	maxInFlight      = 8
	namespaceTimeout = 5 * time.Second
	clusterTimeout   = 15 * time.Second
)

// Fetcher runs a Plan against a Source and folds the answers into a new
// Snapshot.
type Fetcher struct {
	src   Source
	clock func() time.Time
}

// NewFetcher returns a Fetcher over src that reads wall time from the
// system clock.
func NewFetcher(src Source) Fetcher {
	return Fetcher{src: src, clock: time.Now}
}

// WithClock returns a copy of f that reads wall time from clock.
func (f Fetcher) WithClock(clock func() time.Time) Fetcher {
	f.clock = clock
	return f
}

// outcome is one finished request family.
type outcome struct {
	req      Request
	state    State
	err      error
	ns       string // namespace actually listed, after an -A fallback
	fellBack bool
	rows     int
	bytes    int
	requests int
	wall     time.Duration
	apply    func(*store) // writes the decoded payload; nil unless state is OK
}

// Fetch runs p's due requests, at most eight at once, then the typed
// lookups that place newly flagged rows, and returns the next Snapshot.
// Every request runs under its own timeout inside ctx. prev is not changed.
func (f Fetcher) Fetch(ctx context.Context, p Plan, prev Snapshot) Snapshot {
	now := f.clock()
	next := prev
	next.st = prev.st.clone()
	if p.Force {
		next.st.resolved = map[resolveKey]resolveEntry{}
	}
	next.Now, next.At, next.Scope, next.host = p.Now, now, p.Scope, f.src.Host
	next.Requests, next.Bytes = 0, 0

	outs := f.runAll(ctx, p, now)
	for _, o := range outs {
		next = record(next, o, p)
	}
	next = f.place(ctx, next, p, now)
	next.Throttle, next.Failures = backoffCounts(prev, outs)
	return derive(next)
}

func (f Fetcher) runAll(ctx context.Context, p Plan, now time.Time) []outcome {
	outs := make([]outcome, len(p.Due))
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	for i, r := range p.Due {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outs[i] = f.withFallback(ctx, r, p.Scope, now)
		}()
	}
	wg.Wait()
	return outs
}

// withFallback retries a namespaced request denied at -A in the context's
// namespace, as kubectl users would with -n.
func (f Fetcher) withFallback(ctx context.Context, r Request, scope Scope, now time.Time) outcome {
	o := f.timed(ctx, r, now)
	if o.state != StateForbidden || r.Namespace != "" || !r.Endpoint.Namespaced() || scope.Context == "" {
		return o
	}
	retry := r
	retry.Namespace = scope.Context
	o2 := f.timed(ctx, retry, now)
	o2.requests += o.requests
	o2.fellBack = o2.state != StateForbidden
	return o2
}

func (f Fetcher) timed(ctx context.Context, r Request, now time.Time) outcome {
	timeout := clusterTimeout
	if r.Namespace != "" {
		timeout = namespaceTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	o := f.call(cctx, r, now)
	o.wall = time.Since(start)
	o.req, o.ns = r, r.Namespace
	if o.requests == 0 {
		o.requests = 1
	}
	o.state = classify(o.err)
	if o.state != StateOK {
		o.apply = nil
	}
	return o
}

// record folds one outcome into the snapshot: payload, ledger and totals.
func record(s Snapshot, o outcome, p Plan) Snapshot {
	e := o.req.Endpoint
	prev := s.Ledger[e]
	u := Usage{
		Fetched: true, At: p.Now, State: o.state, Namespace: o.ns,
		Rows: o.rows, Bytes: o.bytes, Wall: o.wall, Err: o.err,
	}
	switch {
	case o.state == StateOK && o.apply != nil:
		o.apply(&s.st)
		u.Answered = true
	case o.state == StateOK:
		u.Answered = true
	case o.state.keepsData():
		u.Rows, u.Bytes, u.Answered = prev.Rows, prev.Bytes, prev.Answered
	default:
		clearData(&s.st, e)
	}
	if o.state == StateStale {
		u.StaleSince = p.Now
		if prev.State == StateStale {
			u.StaleSince = prev.StaleSince
		}
	}
	if o.fellBack {
		s.Scope = Scope{Namespace: o.ns, Context: p.Scope.Context, Fallback: true}
	}
	s.Ledger[e] = u
	s.Requests += o.requests
	s.Bytes += o.bytes
	return s
}

// backoffCounts tracks consecutive refreshes with a 429, and consecutive
// refreshes in which every request timed out.
func backoffCounts(prev Snapshot, outs []outcome) (throttle, failures int) {
	if len(outs) == 0 {
		return prev.Throttle, prev.Failures
	}
	allStale := true
	for _, o := range outs {
		if o.state == StateThrottled {
			throttle = prev.Throttle + 1
		}
		allStale = allStale && o.state == StateStale
	}
	if allStale {
		failures = prev.Failures + 1
	}
	return throttle, failures
}

// place runs the typed lookups the current rows need, within the plan's cap.
func (f Fetcher) place(ctx context.Context, s Snapshot, p Plan, now time.Time) Snapshot {
	ws := wanted(s.st, capsOf(s.st).K3s, now)
	todo := pending(s.st, ws, p.ResolveCap)
	for _, r := range f.lookupAll(ctx, todo, p.Force) {
		s.Requests++
		s.Bytes += r.bytes
		if r.keep {
			s.st.resolved[r.key] = r.entry
		}
	}
	s.st.resolved = prune(s.st.resolved, ws)
	return s
}
