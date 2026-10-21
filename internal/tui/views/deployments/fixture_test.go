package deployments_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

const (
	width  = 140
	height = 30

	shop     = "shop"
	hash     = "h1"
	hashKey  = "pod-template-hash"
	revision = "deployment.kubernetes.io/revision"

	appContainer     = tuitest.ContainerApp
	sidecarContainer = tuitest.ContainerSidecar
)

// owned is a deployment in shop as the controllers leave it: the
// deployment, its ReplicaSet of hash h1 and replicas running pods
// <name>-h1-a, -b, ..., each controlled by that ReplicaSet. The resolver
// streams only pods owned this way.
func owned(name string, replicas int32, containers ...string) []runtime.Object {
	const ns = shop
	labels := map[string]string{"app": name}
	dep := tuitest.Deployment(ns, name, replicas, labels, containers...)
	dep.UID = types.UID("uid-" + ns + "-" + name)
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name:            name + "-" + hash,
		Namespace:       ns,
		UID:             types.UID("uid-rs-" + ns + "-" + name),
		Labels:          map[string]string{"app": name, hashKey: hash},
		Annotations:     map[string]string{revision: "1"},
		OwnerReferences: []metav1.OwnerReference{controllerRef("Deployment", name, dep.UID)},
	}}
	objs := make([]runtime.Object, 0, 2+replicas)
	objs = append(objs, dep, rs)
	for i := range replicas {
		pod := tuitest.Pod(ns, podName(name, i), map[string]string{"app": name, hashKey: hash}, containers...)
		pod.OwnerReferences = []metav1.OwnerReference{controllerRef("ReplicaSet", rs.Name, rs.UID)}
		objs = append(objs, pod)
	}
	return objs
}

// podName is the i-th pod of owned's deployment name: <name>-h1-a, -b, ...
func podName(name string, i int32) string {
	return name + "-" + hash + "-" + string(rune('a'+i))
}

func controllerRef(kind, name string, uid types.UID) metav1.OwnerReference {
	yes := true
	return metav1.OwnerReference{APIVersion: "apps/v1", Kind: kind, Name: name, UID: uid, Controller: &yes}
}

// shopObjects is a namespace of deployments sorted api, idle, nosel,
// worker: api runs two single-container pods, worker one pod with a
// sidecar, idle is scaled to 0 and nosel has no selector.
func shopObjects() []runtime.Object {
	noSelector := tuitest.Deployment(shop, "nosel", 1, map[string]string{"app": "nosel"}, appContainer)
	noSelector.Spec.Selector = nil
	return slices.Concat(
		[]runtime.Object{tuitest.Namespace(shop)},
		owned("api", 2, appContainer),
		owned("worker", 1, appContainer, sidecarContainer),
		[]runtime.Object{
			tuitest.Deployment(shop, "idle", 0, map[string]string{"app": "idle"}, appContainer),
			noSelector,
		},
	)
}

// strayObjects is a deployment web in shop whose labels two pods share
// without any controller owning them, as a CronJob's pods would.
func strayObjects() []runtime.Object {
	labels := map[string]string{"app": "web"}
	return []runtime.Object{
		tuitest.Namespace(shop),
		tuitest.Deployment(shop, "web", 2, labels, appContainer),
		tuitest.Pod(shop, "web-1", labels, appContainer),
		tuitest.Pod(shop, "web-2", labels, appContainer),
	}
}

// payObjects is a namespace whose deployments "pay" matches four of:
// pay-api, pay-gateway, pay-worker, payments; web does not match.
func payObjects() []runtime.Object {
	names := []string{"pay-api", "pay-gateway", "pay-worker", "payments", "web"}
	objs := make([]runtime.Object, 0, 1+3*len(names))
	objs = append(objs, tuitest.Namespace(shop))
	for _, name := range names {
		objs = append(objs, owned(name, 1, appContainer)...)
	}
	return objs
}

func newClock() *asynctest.Clock {
	return asynctest.NewClock(time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
}

// mounted is the view sized and loaded, on a manual clock so that no
// tick ever waits on wall time.
func mounted(t *testing.T, c *k8s.Client, namespace string) deployments.Model {
	t.Helper()
	m, _ := mountedOn(t, c, namespace)
	return m
}

func mountedOn(t *testing.T, c *k8s.Client, namespace string) (deployments.Model, *asynctest.Clock) {
	t.Helper()
	clk := newClock()
	m, _ := tuitest.Mount(t, deployments.New(c, namespace, deployments.WithClock(clk)), width, height)
	return m, clk
}

func on(name string, more ...tea.Msg) []tea.Msg {
	return slices.Concat(tuitest.FilterBy(name), more)
}

func statuses(msgs []tea.Msg) []views.StatusMsg {
	return tuitest.All[views.StatusMsg](msgs)
}

// target is what the resolver answers for the deployment in shop, as the
// view should hand it on.
func target(t *testing.T, c *k8s.Client, name string) k8s.LogTarget {
	t.Helper()
	got, err := c.ResolveLogTarget(context.Background(), k8s.LogKindDeployment, shop, name)
	if err != nil {
		t.Fatalf("resolve %s/%s: %v", shop, name, err)
	}
	return got
}

// ref names the deployment in shop as requests carry it.
func ref(name string) views.ResourceRef {
	return views.ResourceRef{Kind: "deployment", Namespace: shop, Name: name}
}

// listRecorder records the ResourceVersion of every deployments list.
type listRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *listRecorder) option() tuitest.FakeOption {
	return tuitest.WithReactor("list", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if l, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
			r.mu.Lock()
			r.seen = append(r.seen, l.GetListOptions().ResourceVersion)
			r.mu.Unlock()
		}
		return false, nil, nil
	})
}

func (r *listRecorder) versions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.seen)
}

// only reports whether want is the one cross-view message in msgs.
func only(msgs []tea.Msg, want tea.Msg) bool {
	got := tuitest.Requests(msgs)
	return len(got) == 1 && tuitest.Contains(got, want)
}
