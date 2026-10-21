package dash

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// bigCluster is a k3s cluster with n pods: 50 per node, a crashing pod in
// every hundred, a pending one in every 150 and n/2 Warning events.
func bigCluster(n int, now time.Time) cluster {
	c := cluster{now: now, git: k3sGit, user: "ops", metrics: true, readyz: readyOK}
	nodes := max(n/50, 3)
	for i := range nodes {
		node := tNode{name: fmt.Sprintf("node-%03d", i), cpu: "8", mem: "32Gi", pods: 110, age: 90 * day, useCPU: "2", useMem: "12Gi"}
		if i == 0 {
			node.roles = []string{"control-plane", "etcd", "master"}
		}
		c.nodes = append(c.nodes, node)
	}
	for i := range 30 {
		c.namespaces = append(c.namespaces, tNS{fmt.Sprintf("team-%02d", i), "Active", 90 * day})
	}
	sys, sysPods := k3sSystem()
	c.workloads, c.pods = sys, sysPods
	for i := range n - len(sysPods) {
		ns, node := fmt.Sprintf("team-%02d", i%30), fmt.Sprintf("node-%03d", i%nodes)
		p := run(ns, fmt.Sprintf("app-%04d-6c9f7d8b5-%05d", i/4, i), node, 5*day)
		switch {
		case i%100 == 7:
			p.status, p.ready, p.restarts, p.lastRestart = "CrashLoopBackOff", 0, 12, 3*minute
			p.conts = []tCont{{name: "app", waiting: "CrashLoopBackOff", restarts: 12}}
		case i%150 == 11:
			p.status, p.ready, p.age = "Pending", 0, 9*minute
		}
		c.pods = append(c.pods, p)
		c.podUse = append(c.podUse, tPodUse{ns, p.name, "25m", "128Mi"})
		if i%4 == 0 {
			c.workloads = append(c.workloads, tWorkload{"deployment", ns, fmt.Sprintf("app-%04d", i/4), 4, 4})
			c.svcs = append(c.svcs, tSvc{ns, fmt.Sprintf("app-%04d", i/4), corev1.ServiceTypeClusterIP, false})
		}
		if i%2 == 0 {
			c.events = append(c.events, tEvent{ns, "Pod", p.name, "", "BackOff", "Back-off pulling image", 3, time.Duration(i%55) * minute})
		}
		if i%10 == 0 {
			c.pvcs = append(c.pvcs, tPVC{ns, fmt.Sprintf("data-%05d", i), "Bound"})
		}
	}
	c.svcs = append(c.svcs, tSvc{"kube-system", "traefik", corev1.ServiceTypeLoadBalancer, false})
	c.helm = []tHelm{{"traefik", "helm-install-traefik"}}
	c.snaps = []tSnap{{"node-000", 4 * hour}}
	return c
}

type tick struct {
	at       time.Duration
	due      []Endpoint
	seen     []seen
	requests int
	bytes    int
	snap     Snapshot
}

// simulate polls a for span of plan time, following each plan's Next.
func simulate(t *testing.T, a *apiserver, span time.Duration, focus Focus) []tick {
	t.Helper()
	f := NewFetcher(a.src).WithClock(func() time.Time { return fixedNow })
	s := NewSnapshot(Scope{})
	var ticks []tick
	for now := time.Duration(0); now < span; {
		p := MakePlan(s.PlanInput(now, true, false, focus))
		s = f.Fetch(context.Background(), p, s)
		log := a.take()
		ticks = append(ticks, tick{at: now, due: dueSet(p), seen: log, requests: s.Requests, bytes: s.Bytes, snap: s})
		if len(log) != s.Requests {
			t.Fatalf("at %v the snapshot counts %d requests, the server saw %d", now, s.Requests, len(log))
		}
		now += p.Next
	}
	return ticks
}

func window(ticks []tick, from, to time.Duration) (requests, bytes int) {
	for _, tk := range ticks {
		if tk.at >= from && tk.at < to {
			requests += tk.requests
			bytes += tk.bytes
		}
	}
	return requests, bytes
}

func TestRequestBudget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pods      int
		cadence   time.Duration
		maxPerMin int
		maxBytes  int // decoded per steady-state minute
		maxPerPod int // bytes per pod row in the pods Table
		firstLoad int
	}{
		{pods: 200, cadence: 5 * sec, maxPerMin: 60, maxBytes: 768 << 10, maxPerPod: 400, firstLoad: 18 + resolveCap},
		{pods: 2000, cadence: 15 * sec, maxPerMin: 60, maxBytes: 3 << 20, maxPerPod: 400, firstLoad: 18 + resolveCap},
		{pods: 5000, cadence: 15 * sec, maxPerMin: 60, maxBytes: 7 << 20, maxPerPod: 400, firstLoad: 18 + resolveCap},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.pods), func(t *testing.T) {
			t.Parallel()
			a := newAPIServer(t, bigCluster(tc.pods, fixedNow))
			ticks := simulate(t, a, 3*time.Minute, watching)

			if got := ticks[0].requests; got > tc.firstLoad {
				t.Errorf("first load made %d requests, want at most %d", got, tc.firstLoad)
			}
			last := ticks[len(ticks)-1].snap
			if p := MakePlan(last.PlanInput(last.Now, true, false, watching)); p.Cadence != tc.cadence {
				t.Errorf("cadence %v, want %v", p.Cadence, tc.cadence)
			}
			reqs, bytes := window(ticks, 2*time.Minute, 3*time.Minute)
			t.Logf("%d pods: first load %d requests %d B; steady minute %d requests %d B (%d ticks); pods Table %d B per pod",
				tc.pods, ticks[0].requests, ticks[0].bytes, reqs, bytes, len(ticks), last.Ledger[Pods].Bytes/tc.pods)
			if reqs > tc.maxPerMin {
				t.Errorf("%d requests in a steady minute, want at most %d", reqs, tc.maxPerMin)
			}
			if bytes > tc.maxBytes {
				t.Errorf("%d bytes in a steady minute, want at most %d", bytes, tc.maxBytes)
			}
			if per := last.Ledger[Pods].Bytes / tc.pods; per > tc.maxPerPod {
				t.Errorf("pods Table costs %d B per pod, want at most %d", per, tc.maxPerPod)
			}
			warnings := (tc.pods - 6) / 2
			if ev := last.Events; ev.Total != warnings || ev.Truncated != (warnings > eventCap) || len(ev.Rows) > maxEventRows {
				t.Errorf("events total %d truncated %v rows %d; want %d warnings", ev.Total, ev.Truncated, len(ev.Rows), warnings)
			}
			for _, tk := range ticks[1:] {
				if slices.Equal(tk.due, []Endpoint{Pods, Deployments}) && tk.requests != 2 {
					t.Errorf("fast-only tick at %v made %d requests, want 2", tk.at, tk.requests)
				}
				checkResourceVersions(t, tk)
			}
		})
	}
}

// A field selector makes the apiserver answer with Continue and no
// remaining count; the capped page must stay capped on every refresh.
func TestCappedEventsStayCappedWithoutARemainingCount(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, bigCluster(4100, fixedNow), withContinueOnly())
	ticks := simulate(t, a, 5*time.Minute, watching)

	var eventLists []seen
	for _, tk := range ticks {
		for _, s := range tk.seen {
			if s.typed && s.verb == "list" && s.resource == "events" {
				eventLists = append(eventLists, s)
			}
		}
	}
	if len(eventLists) < 4 {
		t.Fatalf("only %d event lists in five minutes", len(eventLists))
	}
	for i, s := range eventLists[1:] {
		if s.limit != eventCap {
			t.Errorf("event refresh %d has limit %d, want %d", i+2, s.limit, eventCap)
		}
	}
	if ev := ticks[len(ticks)-1].snap.Events; !ev.Truncated {
		t.Errorf("events not marked truncated: total %d rows %d", ev.Total, len(ev.Rows))
	}
}

// checkResourceVersions: lists read the watch cache (RV=0), except a
// limited count or a capped page, which must read consistently (RV=""),
// since RV=0 ignores limit.
func checkResourceVersions(t *testing.T, tk tick) {
	t.Helper()
	for _, s := range tk.seen {
		if !isList(s) {
			continue
		}
		limited := s.limit > 0
		switch {
		case limited && s.rv != "":
			t.Errorf("at %v %q has limit %d with RV %q", tk.at, s.resource, s.limit, s.rv)
		case !limited && s.rv != "0":
			t.Errorf("at %v %q read RV %q, want 0", tk.at, s.resource, s.rv)
		}
	}
}

// isList: typed lists, and HTTP Table lists (not /readyz or metrics).
func isList(s seen) bool {
	if s.typed {
		return s.verb == "list"
	}
	return s.resource != "/readyz" && !strings.HasPrefix(s.resource, "/apis/metrics.k8s.io/")
}

func TestNoPodMetricsWhilePanelHidden(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, bigCluster(200, fixedNow))
	for _, tk := range simulate(t, a, time.Minute, Focus{Active: true}) {
		for _, s := range tk.seen {
			if strings.HasPrefix(s.resource, "/apis/metrics.k8s.io/v1beta1/") && strings.HasSuffix(s.resource, "/pods") {
				t.Fatalf("pod metrics requested at %v while panel 7 is hidden", tk.at)
			}
		}
	}
}

func TestNoMetricsCallsWhileDiscoveryAbsent(t *testing.T) {
	t.Parallel()

	c := bigCluster(200, fixedNow)
	c.metrics = false
	a := newAPIServer(t, c)
	for _, tk := range simulate(t, a, 2*time.Minute, watching)[1:] {
		for _, s := range tk.seen {
			if strings.HasPrefix(s.resource, "/apis/metrics.k8s.io/") {
				t.Fatalf("metrics requested at %v after discovery said absent", tk.at)
			}
		}
	}
}
