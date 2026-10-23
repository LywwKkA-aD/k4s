package dash

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func fetcher(a *apiserver) Fetcher {
	return NewFetcher(a.src).WithClock(func() time.Time { return fixedNow })
}

func tickAt(t *testing.T, a *apiserver, s Snapshot, now time.Duration) Snapshot {
	t.Helper()
	return fetcher(a).Fetch(context.Background(), MakePlan(s.PlanInput(now, true, false, watching)), s)
}

// typedLists counts typed list calls on resource.
func typedLists(log []seen, resource string) int {
	n := 0
	for _, s := range log {
		if s.typed && s.verb == "list" && s.resource == resource {
			n++
		}
	}
	return n
}

func TestForbiddenIsDataAndParksForFiveMinutes(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	a.failTyped("list nodes", apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", errors.New("RBAC")))
	s := tickAt(t, a, NewSnapshot(Scope{}), 0)
	if s.Nodes.Avail.State != StateForbidden || s.Nodes.Avail.CanI.String() != "kubectl auth can-i list nodes" {
		t.Fatalf("nodes avail = %+v", s.Nodes.Avail)
	}
	if s.Nodes.Total != 0 || len(s.Nodes.Rows) != 0 || !s.Verdict.Partial || s.Verdict.AllClear {
		t.Fatalf("a denied list must not read as data: %+v", s.Verdict)
	}
	a.take()
	s = tickAt(t, a, s, 30*sec)
	if log := a.take(); typedLists(log, "nodes") != 0 {
		t.Fatal("a parked 403 was requested again within five minutes")
	}
	s = tickAt(t, a, s, 5*time.Minute)
	if log := a.take(); typedLists(log, "nodes") != 1 {
		t.Fatal("a parked 403 was not re-probed after five minutes")
	}
	_ = fetcher(a).Fetch(context.Background(), MakePlan(s.PlanInput(5*time.Minute+5*sec, true, true, watching)), s)
	if log := a.take(); typedLists(log, "nodes") != 1 {
		t.Fatal("r did not re-probe the parked endpoint")
	}
}

func TestMetricsStates(t *testing.T) {
	t.Parallel()

	t.Run("503 is not ready, not absent", func(t *testing.T) {
		t.Parallel()
		a := newAPIServer(t, busyK3s(fixedNow))
		a.override("/apis/metrics.k8s.io/", k8stest.ServiceUnavailable())
		s := tickAt(t, a, NewSnapshot(Scope{}), 0)
		if s.Nodes.Metrics.State != StateNotReady || s.TopPods.Avail.State != StateNotReady {
			t.Fatalf("metrics = %v / %v", s.Nodes.Metrics.State, s.TopPods.Avail.State)
		}
		if !s.Caps.Metrics {
			t.Fatal("a 503 must keep polling metrics")
		}
	})
	t.Run("absent from discovery", func(t *testing.T) {
		t.Parallel()
		c := busyK3s(fixedNow)
		c.metrics = false
		s := tickAt(t, newAPIServer(t, c), NewSnapshot(Scope{}), 0)
		if s.Nodes.Metrics.State != StateAbsent || s.TopPods.Avail.State != StateAbsent || s.Caps.Metrics {
			t.Fatalf("metrics = %v / %v caps %v", s.Nodes.Metrics.State, s.TopPods.Avail.State, s.Caps.Metrics)
		}
		if s.Nodes.CPU.Known || s.Nodes.Rows[0].Memory.Known {
			t.Fatal("meters must be unknown without metrics")
		}
	})
	t.Run("pod metrics denied", func(t *testing.T) {
		t.Parallel()
		a := newAPIServer(t, busyK3s(fixedNow))
		a.override("/apis/metrics.k8s.io/v1beta1/pods", k8stest.Forbidden())
		s := tickAt(t, a, NewSnapshot(Scope{}), 0)
		if s.TopPods.Avail.State != StateForbidden || s.TopPods.Avail.CanI.String() != "kubectl auth can-i list pods.metrics.k8s.io -A" {
			t.Fatalf("top pods avail = %+v", s.TopPods.Avail)
		}
	})
}

func TestTimeoutKeepsDataAndMarksStale(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := tickAt(t, a, NewSnapshot(Scope{}), 0)
	total := s.Pods.Total
	a.override("/api/v1/pods", k8stest.Hang())
	for i, now := range []time.Duration{5 * sec, 10 * sec, 75 * sec} {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		s = fetcher(a).Fetch(ctx, MakePlan(s.PlanInput(now, true, false, watching)), s)
		cancel()
		if s.Pods.Avail.State != StateStale || s.Pods.Total != total || s.Ledger[Pods].StaleSince != 5*sec {
			t.Fatalf("refresh %d: avail %+v total %d (want %d) since %v", i, s.Pods.Avail, s.Pods.Total, total, s.Ledger[Pods].StaleSince)
		}
		if s.Pods.Avail.Unanswered {
			t.Fatalf("refresh %d: the kept data counts as unanswered", i)
		}
	}
	if s.Pods.Avail.Stale != time.Minute {
		t.Fatalf("stale for %v, want 1m", s.Pods.Avail.Stale)
	}
}

// TestStaleWithoutAnAnswerHasNoData: a source that timed out before it
// ever answered has nothing to keep, and says so, so its empty panel is
// not read as an empty list.
func TestStaleWithoutAnAnswerHasNoData(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	a.override("/api/v1/pods", k8stest.Hang())
	start := NewSnapshot(Scope{})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	s := fetcher(a).Fetch(ctx, MakePlan(start.PlanInput(0, true, false, watching)), start)
	if s.Pods.Avail.State != StateStale || !s.Pods.Avail.Unanswered {
		t.Fatalf("pods = %+v, want stale and unanswered", s.Pods.Avail)
	}
	if s.Nodes.Avail.State != StateOK || s.Nodes.Avail.Unanswered || !s.Ledger[Nodes].Answered {
		t.Fatalf("nodes = %+v (answered %v), want an answer", s.Nodes.Avail, s.Ledger[Nodes].Answered)
	}
	if again := s.Rescope(Scope{Namespace: "shop"}); again.Ledger[Pods].Answered {
		t.Fatal("a new scope must start unanswered")
	}
}

func TestThrottleBacksOffTheWholePoller(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	a.failTyped("list events", apierrors.NewTooManyRequests("slow down", 1))
	s := tickAt(t, a, NewSnapshot(Scope{}), 0)
	if s.Throttle != 1 {
		t.Fatalf("throttle = %d", s.Throttle)
	}
	if p := MakePlan(s.PlanInput(0, true, false, watching)); p.Next != 10*sec || !p.Slowed {
		t.Fatalf("plan after a 429: next %v slowed %v", p.Next, p.Slowed)
	}
	s = tickAt(t, a, s, 10*sec)
	if s.Throttle != 0 {
		t.Fatalf("a clean tick must end the backoff: throttle %d", s.Throttle)
	}
}

func TestOldServersFallBackToTheServicesTable(t *testing.T) {
	t.Parallel()

	t.Run("a 400 on the selector", func(t *testing.T) {
		t.Parallel()
		a := newAPIServer(t, vanillaKind(fixedNow))
		a.failTyped("list services", apierrors.NewBadRequest(`Unable to find "/v1, Resource=services" that match label selector "", field selector "spec.type=LoadBalancer": field label not supported: spec.type`))
		s := tickAt(t, a, NewSnapshot(Scope{}), 0)
		if !s.Caps.LBFallback || s.Ledger[LoadBalancers].State != StateOK {
			t.Fatalf("caps %+v ledger %+v", s.Caps, s.Ledger[LoadBalancers])
		}
		n := s.Cluster.Network
		if n.Pending != 1 || n.Services != 2 || !n.Cmd.Approx || n.Cmd.String() != "kubectl get svc -A" {
			t.Fatalf("network = %+v (%s)", n, n.Cmd)
		}
		p := MakePlan(s.PlanInput(60*sec, true, false, watching))
		if slices.Contains(dueSet(p), ServiceCount) {
			t.Fatal("the Table fallback already counts services")
		}
	})
	t.Run("a version before 1.31", func(t *testing.T) {
		t.Parallel()
		c := vanillaKind(fixedNow)
		c.git = "v1.29.4"
		s := tickAt(t, newAPIServer(t, c), NewSnapshot(Scope{}), 0)
		if !s.Caps.LBFallback {
			t.Fatal("a 1.29 server must use the services Table")
		}
		p := MakePlan(s.PlanInput(30*sec, true, false, watching))
		for _, r := range p.Due {
			if r.Endpoint == LoadBalancers && !r.Fallback {
				t.Fatal("LoadBalancers still uses the field selector")
			}
		}
	})
}

func TestScopeFallbackWithoutAContextNamespace(t *testing.T) {
	t.Parallel()

	s := tickAt(t, newAPIServer(t, nsScopedEKS(fixedNow), scopedTo("shop")), NewSnapshot(Scope{}), 0)
	if s.Scope.Fallback || s.Pods.Avail.State != StateForbidden {
		t.Fatalf("scope %+v pods %+v", s.Scope, s.Pods.Avail)
	}
	if got := s.Pods.Avail.CanI.String(); got != "kubectl auth can-i list pods -A" {
		t.Fatalf("can-i = %q", got)
	}
	if len(s.Next) == 0 || s.Next[0].Cmd.String() != "kubectl auth can-i --list" {
		t.Fatalf("next = %v", s.Next)
	}
}

func TestSecretsNeverReachASnapshot(t *testing.T) {
	t.Parallel()

	s := refresh(t, newAPIServer(t, busyK3s(fixedNow)), Scope{})
	dump := fmt.Sprintf("%#v", s)
	for _, secret := range []string{"SECRET-TOKEN-X", "node-args", "extra-must-not-show"} {
		if strings.Contains(dump, secret) {
			t.Errorf("snapshot carries %q", secret)
		}
	}
}

func TestForceReadsConsistentlyAndRedoesLookups(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := refresh(t, a, Scope{})
	a.take()
	s = fetcher(a).Fetch(context.Background(), MakePlan(s.PlanInput(12*sec, true, true, watching)), s)
	log := a.take()
	lookups := 0
	for _, r := range log {
		if r.typed && strings.HasPrefix(r.fieldSelector, "metadata.name=") {
			lookups++
		}
		if r.hasRV && r.rv != "" {
			t.Errorf("r: %s %q read RV %q", r.verb, r.resource, r.rv)
		}
	}
	if lookups != resolveCap {
		t.Fatalf("r redid %d lookups, want %d", lookups, resolveCap)
	}
	if s.Requests != len(log) {
		t.Fatalf("snapshot counts %d requests, server saw %d", s.Requests, len(log))
	}
}

func TestFetchLeavesThePreviousSnapshotAlone(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	first := tickAt(t, a, NewSnapshot(Scope{}), 0)
	before := fmt.Sprintf("%#v", first)
	resolved := len(first.st.resolved)
	_ = tickAt(t, a, first, 5*sec)
	if after := fmt.Sprintf("%#v", first); after != before || len(first.st.resolved) != resolved {
		t.Fatal("Fetch changed the snapshot it was given")
	}
}

func TestRescopeRefetchesNamespacedData(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := refresh(t, a, Scope{})
	pods := s.Pods.Total
	shop := s.Rescope(Scope{Namespace: "shop", Context: "shop"})
	if shop.Pods.Total != 0 || shop.Ledger[Pods].Fetched || shop.Nodes.Total != 3 || s.Pods.Total != pods {
		t.Fatalf("rescope: pods %d fetched %v nodes %d; original pods %d", shop.Pods.Total, shop.Ledger[Pods].Fetched, shop.Nodes.Total, s.Pods.Total)
	}
	p := MakePlan(shop.PlanInput(12*sec, true, false, watching))
	for _, r := range p.Due {
		if r.Endpoint.Namespaced() && r.Namespace != "shop" {
			t.Fatalf("%v still lists %q", r.Endpoint, r.Namespace)
		}
	}
	if !slices.Contains(dueSet(p), Events) || slices.Contains(dueSet(p), Nodes) {
		t.Fatalf("due after rescope = %v", dueSet(p))
	}
	shop = fetcher(a).Fetch(context.Background(), p, shop)
	if shop.Pods.Total != 4 || shop.Pods.Cmd.String() != "kubectl get pods -n shop" {
		t.Fatalf("shop pods = %d (%s)", shop.Pods.Total, shop.Pods.Cmd)
	}
}

func TestUnchangedPollsHashTheSame(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow))
	s := refresh(t, a, Scope{})
	again := tickAt(t, a, s, 15*sec)
	if again.Hash != s.Hash {
		t.Fatalf("an unchanged poll changed hashes:\n%+v\n%+v", s.Hash, again.Hash)
	}
	later := NewFetcher(a.src).WithClock(func() time.Time { return fixedNow.Add(20 * time.Second) })
	moved := later.Fetch(context.Background(), MakePlan(again.PlanInput(20*sec, true, false, watching)), again)
	if moved.Hash.Workloads != again.Hash.Workloads || moved.Hash.Cluster != again.Hash.Cluster {
		t.Fatal("twenty seconds of clock alone changed a section hash")
	}
}

func TestFailingReadyzIsData(t *testing.T) {
	t.Parallel()

	c := vanillaKind(fixedNow)
	c.readyz = "[+]ping ok\n[-]etcd failed: reason withheld\n[+]log ok\nreadyz check failed\n"
	s := tickAt(t, newAPIServer(t, c), NewSnapshot(Scope{}), 0)
	api := s.Cluster.API
	if api.Avail.State != StateOK || api.Ready || api.Etcd != StateFailed || api.Sev != SevFailing || !slices.Equal(api.Failed, []string{"etcd"}) {
		t.Fatalf("api = %+v", api)
	}
	if s.Verdict.AllClear || s.Verdict.Chips[0].Text != "api not ready" {
		t.Fatalf("verdict = %s", s.Verdict)
	}
}

func TestEtcdRowFallsBackToTheReconcileAnnotation(t *testing.T) {
	t.Parallel()

	c := busyK3s(fixedNow)
	c.snaps = nil
	s := refresh(t, newAPIServer(t, c), Scope{})
	e := s.K3s.Etcd
	if !e.Shown || !e.Reconciled || !e.Known || e.Node != "k3s-cp-1" || e.Ago != 10*time.Hour {
		t.Fatalf("etcd row = %+v", e)
	}
	want := `kubectl get node k3s-cp-1 -o 'jsonpath={.metadata.annotations.etcd\.k3s\.cattle\.io/local-snapshots-timestamp}'`
	if got := e.Cmd.String(); got != want {
		t.Fatalf("etcd cmd = %s", got)
	}
}

func TestNewSource(t *testing.T) {
	t.Parallel()

	if _, err := NewSource(nil); !errors.Is(err, ErrNoClient) {
		t.Fatalf("nil client: %v", err)
	}
	fakeClient, _ := k8stest.NewFakeClient()
	src, err := NewSource(fakeClient)
	if err != nil || src.Typed == nil || usable(src.REST) || src.Host != "" {
		t.Fatalf("fake source = %+v, %v", src, err)
	}
	served := k8stest.NewServerClient(t, k8stest.NotFound())
	src, err = NewSource(served)
	if err != nil || src.Host != served.RestConfig.Host || !usable(src.REST) || src.Typed == served.Clientset {
		t.Fatalf("server source = %+v, %v", src, err)
	}
	// A fake with a server address (as the header tests use) must stay the
	// fake: a protobuf copy of the config would dial that address.
	named, _ := k8stest.NewFakeClient()
	named.RestConfig = &rest.Config{Host: "https://10.0.0.5:6443"}
	src, err = NewSource(named)
	if err != nil || src.Typed != named.Clientset || usable(src.REST) || src.Host != "https://10.0.0.5:6443" {
		t.Fatalf("named fake source = %+v, %v", src, err)
	}
}

func TestFakeSourceWithoutRESTDegradesTablePanels(t *testing.T) {
	t.Parallel()

	fakeClient, _ := k8stest.NewFakeClient()
	src, err := NewSource(fakeClient)
	if err != nil {
		t.Fatal(err)
	}
	s := NewFetcher(src).Fetch(context.Background(), MakePlan(PlanInput{Watch: true, Focus: watching}), NewSnapshot(Scope{}))
	if s.Pods.Avail.State != StateAbsent || s.Nodes.Avail.State != StateOK {
		t.Fatalf("pods %v nodes %v", s.Pods.Avail.State, s.Nodes.Avail.State)
	}
}
