package dashboard

import (
	"context"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// FetchFunc runs one refresh: the requests p makes due, folded into the
// snapshot after prev. dash.Fetcher.Fetch is the production one; tests
// pass their own through WithFetch.
type FetchFunc func(ctx context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot

// Option configures New.
type Option func(config) config

type config struct {
	ns      string
	mem     Memory
	blurred bool
	now     func() time.Time
	clock   async.Clock
	fetch   FetchFunc
	session kubectl.Session
}

// WithSession is how a pasted command reaches the cluster k4s shows (see
// kubectl.Decorate). The root passes its own, so the commands the body
// prints for copying (the next suggestion, a denied source's probe) and
// the help map carry the same --context and --kubeconfig as the footer.
func WithSession(s kubectl.Session) Option {
	return func(c config) config {
		c.session = s
		return c
	}
}

// WithNamespace is the namespace the dashboard lists, "" for all; the
// root passes its active one.
func WithNamespace(ns string) Option {
	return func(c config) config {
		c.ns = ns
		return c
	}
}

// WithMemory hands back what an earlier dashboard learned (see Memory).
// Memory taken for another client is dropped, and memory of another
// namespace is rescoped.
func WithMemory(mem Memory) Option {
	return func(c config) config {
		c.mem = mem
		return c
	}
}

// WithBlurred starts the dashboard knowing the terminal does not have
// focus, as the last BlurMsg said, so it polls at the blurred cadence.
func WithBlurred(blurred bool) Option {
	return func(c config) config {
		c.blurred = blurred
		return c
	}
}

// WithClock makes the dashboard read wall time from now instead of
// time.Now: plan time, idle time and history all follow it.
func WithClock(now func() time.Time) Option {
	return func(c config) config {
		c.now = now
		return c
	}
}

// WithTicker arms the refresh ticks on clock instead of tea.Tick, so tests
// fire them by hand (asynctest.Clock).
func WithTicker(clock async.Clock) Option {
	return func(c config) config {
		c.clock = clock
		return c
	}
}

// WithFetch replaces the refresh, so tests answer with fixtures instead of
// a cluster. It applies only with a client.
func WithFetch(f FetchFunc) Option {
	return func(c config) config {
		c.fetch = f
		return c
	}
}

func newConfig(opts []Option) config {
	c := config{now: time.Now}
	for _, o := range opts {
		c = o(c)
	}
	return c
}
