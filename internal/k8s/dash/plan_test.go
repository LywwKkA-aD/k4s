package dash

import (
	"slices"
	"testing"
	"time"
)

const sec = time.Second

// fetchedAt marks every endpoint in eps as fetched OK at plan time at.
func fetchedAt(at time.Duration, eps ...Endpoint) Ledger {
	var l Ledger
	for _, e := range eps {
		l[e] = Usage{Fetched: true, At: at, State: StateOK}
	}
	return l
}

func allFetched() Ledger { return fetchedAt(0, Endpoints()...) }

func allFetchedExcept(at time.Duration, skip ...Endpoint) Ledger {
	return fetchedAt(at, slices.DeleteFunc(Endpoints(), func(e Endpoint) bool { return slices.Contains(skip, e) })...)
}

func active() Focus { return Focus{Active: true} }

func dueSet(p Plan) []Endpoint {
	out := make([]Endpoint, 0, len(p.Due))
	for _, r := range p.Due {
		out = append(out, r.Endpoint)
	}
	return out
}

func TestMakePlanDueSets(t *testing.T) {
	t.Parallel()

	steady := Caps{Metrics: true}
	cases := []struct {
		name string
		in   PlanInput
		want []Endpoint
	}{
		{
			name: "first load asks every tier, metrics optimistically, k3s extras not yet",
			in:   PlanInput{Watch: true, Focus: active()},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets, Nodes, Events, PVCs,
				Jobs, CronJobs, LoadBalancers, ServiceCount, Readyz, Namespaces, Version, Whoami, MetricsDiscovery},
		},
		{
			name: "first load with panel 7 visible adds pod metrics",
			in:   PlanInput{Watch: true, Focus: Focus{Active: true, TopPods: true}},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets, PodMetrics, Nodes, Events, PVCs,
				Jobs, CronJobs, LoadBalancers, ServiceCount, Readyz, Namespaces, Version, Whoami, MetricsDiscovery},
		},
		{
			name: "a fast-only tick is pods and deployments",
			in:   PlanInput{Now: 5 * sec, Watch: true, Focus: active(), Ledger: allFetched(), Caps: steady},
			want: []Endpoint{Pods, Deployments},
		},
		{
			name: "every third tick adds the medium tier",
			in:   PlanInput{Now: 15 * sec, Watch: true, Focus: active(), Ledger: allFetched(), Caps: steady},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets},
		},
		{
			name: "every sixth tick adds the slow tier; the service count waits for 60s",
			in:   PlanInput{Now: 30 * sec, Watch: true, Focus: active(), Ledger: allFetched(), Caps: steady},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets, Nodes, Events, PVCs,
				Jobs, CronJobs, LoadBalancers, Readyz, Namespaces},
		},
		{
			name: "pod metrics only while panel 7 is visible",
			in: PlanInput{Now: 15 * sec, Watch: true, Focus: Focus{Active: true, TopPods: true},
				Ledger: allFetched(), Caps: steady},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets, PodMetrics},
		},
		{
			name: "no metrics calls while discovery says absent",
			in: PlanInput{Now: 15 * sec, Watch: true, Focus: Focus{Active: true, TopPods: true},
				Ledger: allFetched()},
			want: []Endpoint{Pods, Deployments, StatefulSets, DaemonSets},
		},
		{
			name: "absent metrics are re-discovered every 60s",
			in:   PlanInput{Now: 60 * sec, Watch: true, Focus: active(), Ledger: allFetched()},
			want: []Endpoint{Pods, Deployments, StatefulSets, DaemonSets, Nodes, Events, PVCs, Jobs, CronJobs,
				LoadBalancers, ServiceCount, Readyz, Namespaces, MetricsDiscovery},
		},
		{
			name: "k3s adds helmcharts; etcd adds snapshot files once known",
			in: PlanInput{Now: 5 * sec, Watch: true, Focus: active(), Ledger: allFetchedExcept(0, HelmCharts, EtcdSnapshots),
				Caps: Caps{Metrics: true, K3s: true, Etcd: true}},
			want: []Endpoint{Pods, Deployments, HelmCharts, EtcdSnapshots},
		},
		{
			name: "old servers list services as a Table and need no separate count",
			in: PlanInput{Now: 60 * sec, Watch: true, Focus: active(), Ledger: allFetched(),
				Caps: Caps{Metrics: true, LBFallback: true}},
			want: []Endpoint{Pods, Deployments, NodeMetrics, StatefulSets, DaemonSets, Nodes, Events, PVCs,
				Jobs, CronJobs, LoadBalancers, Readyz, Namespaces},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := dueSet(MakePlan(tc.in)); !slices.Equal(got, tc.want) {
				t.Fatalf("due = %v\nwant  %v", got, tc.want)
			}
		})
	}
}

func TestMakePlanArmsOnlyWhenWatchingAnActiveView(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		watch bool
		focus Focus
		arm   bool
	}{
		{"watching the active dashboard", true, active(), true},
		{"watch off", false, active(), false},
		{"dashboard not the current view", true, Focus{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := MakePlan(PlanInput{Now: 5 * sec, Watch: tc.watch, Focus: tc.focus, Ledger: allFetched()})
			if p.Arm != tc.arm {
				t.Fatalf("Arm = %v, want %v", p.Arm, tc.arm)
			}
			if !p.Arm && p.Next != 0 {
				t.Fatalf("an unarmed plan has Next %v", p.Next)
			}
		})
	}
}

func TestMakePlanNextInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      PlanInput
		next    time.Duration
		cadence time.Duration
		slowed  bool
	}{
		{"steady 5s", PlanInput{Ledger: allFetched()}, 5 * sec, 5 * sec, false},
		{"blur stretches the fast tier to 15s", PlanInput{Focus: Focus{Blurred: true}, Ledger: allFetched()}, 15 * sec, 15 * sec, false},
		{"ten idle minutes stretch the fast tier", PlanInput{Focus: Focus{Idle: 10 * time.Minute}, Ledger: allFetched()}, 15 * sec, 15 * sec, false},
		{"over 1000 pod rows", PlanInput{Ledger: withPods(allFetched(), 1500, 0, 0)}, 15 * sec, 15 * sec, true},
		{"over 5000 pod rows", PlanInput{Ledger: withPods(allFetched(), 6000, 0, 0)}, 30 * sec, 30 * sec, true},
		{"a pods list slower than 1s", PlanInput{Ledger: withPods(allFetched(), 100, 0, 1500*time.Millisecond)}, 15 * sec, 15 * sec, true},
		{"a pods body over 4 MiB", PlanInput{Ledger: withPods(allFetched(), 100, 5<<20, 0)}, 30 * sec, 30 * sec, true},
		{"one 429 doubles the interval", PlanInput{Throttle: 1, Ledger: allFetched()}, 10 * sec, 5 * sec, true},
		{"429s cap at 60s", PlanInput{Throttle: 6, Ledger: allFetched()}, 60 * sec, 5 * sec, true},
		{"timeouts back off 10s", PlanInput{Failures: 1, Ledger: allFetched()}, 10 * sec, 5 * sec, true},
		{"timeouts back off 40s", PlanInput{Failures: 3, Ledger: allFetched()}, 40 * sec, 5 * sec, true},
		{"timeouts cap at 60s", PlanInput{Failures: 9, Ledger: allFetched()}, 60 * sec, 5 * sec, true},
		{"a first load waits a full fast interval", PlanInput{Focus: Focus{Blurred: true}}, 15 * sec, 15 * sec, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := tc.in
			in.Watch, in.Focus.Active = true, true
			p := MakePlan(in)
			if p.Next != tc.next || p.Cadence != tc.cadence || p.Slowed != tc.slowed {
				t.Fatalf("Next %v Cadence %v Slowed %v; want %v %v %v", p.Next, p.Cadence, p.Slowed, tc.next, tc.cadence, tc.slowed)
			}
		})
	}
}

func withPods(l Ledger, rows, bytes int, wall time.Duration) Ledger {
	l[Pods] = Usage{Fetched: true, State: StateOK, Rows: rows, Bytes: bytes, Wall: wall}
	return l
}

func TestMakePlanParksDeniedAndAbsentEndpoints(t *testing.T) {
	t.Parallel()

	denied := allFetched()
	denied[Nodes] = Usage{Fetched: true, State: StateForbidden}
	denied[HelmCharts] = Usage{Fetched: true, State: StateAbsent, Namespace: ""}
	denied[Events] = Usage{Fetched: true, State: StateForbidden, Namespace: ""}
	k3s := Caps{Metrics: true, K3s: true}

	cases := []struct {
		name  string
		in    PlanInput
		want  []Endpoint
		avoid []Endpoint
	}{
		{"parked inside five minutes", PlanInput{Now: 30 * sec, Ledger: denied, Caps: k3s}, nil, []Endpoint{Nodes, HelmCharts, Events}},
		{"re-probed after five minutes", PlanInput{Now: 5 * time.Minute, Ledger: denied, Caps: k3s}, []Endpoint{Nodes, HelmCharts, Events}, nil},
		{"re-probed on r", PlanInput{Now: 30 * sec, Force: true, Ledger: denied, Caps: k3s}, []Endpoint{Nodes, HelmCharts, Events}, nil},
		{"a new namespace is a new key", PlanInput{Now: 30 * sec, Ledger: denied, Caps: k3s, Scope: Scope{Namespace: "shop"}}, []Endpoint{Events}, []Endpoint{Nodes}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := tc.in
			in.Watch, in.Focus = true, active()
			got := dueSet(MakePlan(in))
			for _, e := range tc.want {
				if !slices.Contains(got, e) {
					t.Errorf("%v not due: %v", e, got)
				}
			}
			for _, e := range tc.avoid {
				if slices.Contains(got, e) {
					t.Errorf("%v due while parked: %v", e, got)
				}
			}
		})
	}
}

// Ticks read the watch cache; counts and r read consistently.
func TestMakePlanResourceVersions(t *testing.T) {
	t.Parallel()

	tick := MakePlan(PlanInput{Watch: true, Focus: active()})
	for _, r := range tick.Due {
		want := r.Endpoint == ServiceCount
		if r.Consistent != want {
			t.Errorf("%v Consistent = %v, want %v", r.Endpoint, r.Consistent, want)
		}
	}
	force := MakePlan(PlanInput{Now: 7 * sec, Force: true, Watch: true, Focus: active(), Ledger: allFetched()})
	if !force.Force || force.ResolveCap != resolveCap {
		t.Fatalf("force plan = %+v", force)
	}
	for _, r := range force.Due {
		if !r.Consistent {
			t.Errorf("r: %v reads the watch cache", r.Endpoint)
		}
	}
}

func TestMakePlanScopeMovesOnlyNamespacedEndpoints(t *testing.T) {
	t.Parallel()

	p := MakePlan(PlanInput{Watch: true, Focus: active(), Scope: Scope{Namespace: "shop"}})
	for _, r := range p.Due {
		want := ""
		if r.Endpoint.Namespaced() {
			want = "shop"
		}
		if r.Namespace != want {
			t.Errorf("%v namespace %q, want %q", r.Endpoint, r.Namespace, want)
		}
	}
	if p.Scope.Namespace != "shop" {
		t.Fatalf("plan scope = %+v", p.Scope)
	}
}

func TestMakePlanRequestShapes(t *testing.T) {
	t.Parallel()

	t.Run("big nodes and events lists slow down and events get a cap", func(t *testing.T) {
		t.Parallel()
		l := allFetched()
		l[Nodes] = Usage{Fetched: true, State: StateOK, Rows: 21}
		l[Events] = Usage{Fetched: true, State: StateOK, Rows: 2500}
		at30 := dueSet(MakePlan(PlanInput{Now: 30 * sec, Watch: true, Focus: active(), Ledger: l}))
		if slices.Contains(at30, Nodes) || slices.Contains(at30, Events) {
			t.Fatalf("big nodes or events due at 30s: %v", at30)
		}
		p := MakePlan(PlanInput{Now: 60 * sec, Watch: true, Focus: active(), Ledger: l})
		var ev Request
		for _, r := range p.Due {
			if r.Endpoint == Events {
				ev = r
			}
		}
		if ev.Limit != eventCap || !ev.Consistent {
			t.Fatalf("capped events request = %+v", ev)
		}
		if !slices.Contains(dueSet(p), Nodes) {
			t.Fatalf("nodes not due at 60s: %v", dueSet(p))
		}
	})

	t.Run("pod metrics slow to 30s above 1000 pods", func(t *testing.T) {
		t.Parallel()
		l := withPods(allFetched(), 1200, 0, 0)
		in := PlanInput{Now: 15 * sec, Watch: true, Focus: Focus{Active: true, TopPods: true}, Ledger: l, Caps: Caps{Metrics: true}}
		if slices.Contains(dueSet(MakePlan(in)), PodMetrics) {
			t.Fatal("pod metrics due at 15s above 1000 pods")
		}
		in.Now = 30 * sec
		if !slices.Contains(dueSet(MakePlan(in)), PodMetrics) {
			t.Fatal("pod metrics not due at 30s")
		}
	})
}

func TestMakePlanOldServersListTheServicesTable(t *testing.T) {
	t.Parallel()

	p := MakePlan(PlanInput{Watch: true, Focus: active(), Caps: Caps{LBFallback: true}})
	for _, r := range p.Due {
		if r.Endpoint == LoadBalancers && !r.Fallback {
			t.Fatal("LoadBalancers request is not the fallback")
		}
	}
}

func TestMakePlanRetriesOnceTierAfterTransientFailure(t *testing.T) {
	t.Parallel()

	for _, st := range []State{StateStale, StateFailed, StateThrottled, StateNotReady, StateUnauthorized} {
		t.Run(st.String(), func(t *testing.T) {
			t.Parallel()
			l := allFetched()
			l[Version] = Usage{Fetched: true, At: 0, State: st}
			p := MakePlan(PlanInput{Now: 5 * sec, Watch: true, Focus: active(), Ledger: l, Caps: Caps{Metrics: true}})
			if !slices.Contains(dueSet(p), Version) {
				t.Fatalf("Version not retried after %v: %v", st, dueSet(p))
			}
		})
	}
	for _, st := range []State{StateOK, StateForbidden, StateAbsent, StateUnsupported} {
		t.Run("final "+st.String(), func(t *testing.T) {
			t.Parallel()
			l := allFetched()
			l[Whoami] = Usage{Fetched: true, At: 0, State: st}
			p := MakePlan(PlanInput{Now: 30 * sec, Watch: true, Focus: active(), Ledger: l, Caps: Caps{Metrics: true}})
			if slices.Contains(dueSet(p), Whoami) {
				t.Fatalf("Whoami retried after %v", st)
			}
		})
	}
}
