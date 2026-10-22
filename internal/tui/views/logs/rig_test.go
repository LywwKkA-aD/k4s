package logs

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The rig drives a logs view against a fake cluster with a manual clock.
// Its streams stay open and say nothing: the test hands the view the
// lines and ends a real pump would, so every step is deterministic.

const shop = "shop"

// quick runs the view's Cmds against the fake API, which answers at once;
// a read of the (silent) streams is given up quickly.
var quick = tuitest.Driver{CmdTimeout: 30 * time.Millisecond}

// patient runs what the clock's timers bring: re-resolves, checks,
// reconnects and the timers they arm. None of them reads a stream, so
// they all return, and a loaded machine cannot make the driver give up on
// a reply or on a timer before it is armed.
var patient = tuitest.Driver{CmdTimeout: 10 * time.Second}

// recorder is a streamer whose streams stay open until the session stops
// and never send a line. It records how each pod's log was asked for.
type recorder struct {
	mu    sync.Mutex
	asked map[string][]k8s.LogOptions
}

func (r *recorder) StreamLogs(ctx context.Context, _, pod string, opts k8s.LogOptions) (io.ReadCloser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asked == nil {
		r.asked = map[string][]k8s.LogOptions{}
	}
	r.asked[pod] = append(r.asked[pod], opts)
	return io.NopCloser(untilDone{ctx}), nil
}

// opened waits for pod's n-th stream to open and returns how it was asked.
func (r *recorder) opened(t *testing.T, pod string, n int) k8s.LogOptions {
	t.Helper()
	var got []k8s.LogOptions
	tuitest.Eventually(t, 2*time.Second, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		got = slices.Clone(r.asked[pod])
		return len(got) >= n
	}, "pod "+pod+" was not streamed")
	return got[n-1]
}

func (r *recorder) count(pod string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked[pod])
}

type rig struct {
	t     *testing.T
	m     Model
	clock *asynctest.Clock
	src   *recorder
	cs    *fake.Clientset
	out   []tea.Msg
}

// newRig opens the logs of pods; with a target, of the deployment or
// service it names, resolved as the deployments view does.
func newRig(t *testing.T, objs []runtime.Object, kind, name string, pods ...string) *rig {
	t.Helper()
	c, cs := k8stest.NewFakeClient(objs...)
	var opts []Option
	if kind != "" {
		target, err := c.ResolveLogTarget(context.Background(), kind, shop, name)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		pods = target.PodNames()
		opts = append(opts, WithTarget(target))
	}
	return mount(t, c, cs, pods, opts...)
}

func mount(t *testing.T, c *k8s.Client, cs *fake.Clientset, pods []string, opts ...Option) *rig {
	t.Helper()
	r := &rig{t: t, clock: asynctest.NewClock(time.Unix(1_700_000_000, 0)), src: &recorder{}, cs: cs}
	opts = append(opts, WithClock(r.clock), withSource(r.src))
	m := New(c, shop, pods, 100, "", opts...)
	t.Cleanup(func() { _ = m.Close() })
	m, _ = tuitest.DriveWith(t, quick, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m, r.out = tuitest.InitWith(t, quick, m)
	cs.ClearActions()
	r.m = m
	return r
}

// send delivers msgs and keeps what they bring.
func (r *rig) send(msgs ...tea.Msg) {
	r.t.Helper()
	r.sendWith(quick, msgs...)
}

func (r *rig) sendWith(d tuitest.Driver, msgs ...tea.Msg) {
	r.t.Helper()
	var out []tea.Msg
	r.m, out = tuitest.DriveWith(r.t, d, r.m, msgs...)
	r.out = append(r.out, out...)
}

// advance moves the clock by d and delivers the timers due.
func (r *rig) advance(d time.Duration) {
	r.t.Helper()
	r.sendWith(patient, r.clock.Advance(d)...)
}

// feed hands the view one batch, as the pump would.
func (r *rig) feed(events ...logEvent) {
	r.t.Helper()
	r.send(logEventsBatch{tok: r.m.sess.tok, events: events})
}

// line is a line of pod's current stream stamped ts.
func (r *rig) line(pod string, ts time.Time, text string) logEvent {
	return logEvent{Pod: pod, Attempt: r.m.streams.pods[pod].attempt, Line: text, TS: ts}
}

// end is the end of pod's current stream with err (nil: end of log).
func (r *rig) end(pod string, err error) logEvent {
	return logEvent{Pod: pod, Attempt: r.m.streams.pods[pod].attempt, Done: true, Err: err}
}

func (r *rig) screen() string { return tuitest.Screen(r.m) }

func (r *rig) shows(want ...string) {
	r.t.Helper()
	screen := r.screen()
	for _, w := range want {
		if !strings.Contains(screen, w) {
			r.t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
}

func (r *rig) statuses() []string {
	all := tuitest.All[views.StatusMsg](r.out)
	out := make([]string, 0, len(all))
	for _, s := range all {
		out = append(out, s.Text)
	}
	return out
}

// calls counts the API calls since the view opened, as "verb resource".
func (r *rig) calls() map[string]int {
	out := map[string]int{}
	for _, a := range r.cs.Actions() {
		out[a.GetVerb()+" "+a.GetResource().Resource]++
	}
	return out
}

// mutate changes a pod in the fake cluster.
func (r *rig) mutate(name string, change func(*corev1.Pod)) {
	r.t.Helper()
	pod, err := r.cs.CoreV1().Pods(shop).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		r.t.Fatalf("get %s: %v", name, err)
	}
	change(pod)
	if _, err := r.cs.CoreV1().Pods(shop).Update(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		r.t.Fatalf("update %s: %v", name, err)
	}
}

func (r *rig) add(objs ...runtime.Object) {
	r.t.Helper()
	for _, obj := range objs {
		if err := r.cs.Tracker().Add(obj); err != nil {
			r.t.Fatalf("add: %v", err)
		}
	}
}

func (r *rig) remove(pods ...string) {
	r.t.Helper()
	for _, p := range pods {
		if err := r.cs.CoreV1().Pods(shop).Delete(context.Background(), p, metav1.DeleteOptions{}); err != nil {
			r.t.Fatalf("delete %s: %v", p, err)
		}
	}
}

// Fixtures: the deployment web in shop, its ReplicaSets and their pods.

var created = metav1.NewTime(time.Unix(1_700_000_000, 0))

func webDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: shop, UID: "uid-web"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
		},
	}
}

func webReplicaSet(hash, revision string) *appsv1.ReplicaSet {
	yes := true
	return &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "web-" + hash, Namespace: shop, UID: types.UID("uid-" + hash),
		Labels:          map[string]string{"app": "web", "pod-template-hash": hash},
		Annotations:     map[string]string{"deployment.kubernetes.io/revision": revision},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", UID: "uid-web", Controller: &yes}},
	}}
}

// webPod is a running pod of ReplicaSet hash, created at sec past created.
func webPod(name, hash string, sec int64) *corev1.Pod {
	yes := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: shop, CreationTimestamp: metav1.NewTime(created.Add(time.Duration(sec) * time.Second)),
			Labels:          map[string]string{"app": "web", "pod-template-hash": hash},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-" + hash, UID: types.UID("uid-" + hash), Controller: &yes}},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}},
		}}},
	}
}

// pending makes p a pod whose container is being created.
func pending(p *corev1.Pod) *corev1.Pod {
	p.Status.Phase = corev1.PodPending
	p.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}
	return p
}

func webObjects(pods ...*corev1.Pod) []runtime.Object {
	objs := make([]runtime.Object, 0, 2+len(pods))
	objs = append(objs, webDeployment(), webReplicaSet("h1", "1"))
	for _, p := range pods {
		objs = append(objs, p)
	}
	return objs
}

func k8stestClient(pods ...*corev1.Pod) (*k8s.Client, *fake.Clientset) {
	objs := make([]runtime.Object, 0, len(pods))
	for _, p := range pods {
		objs = append(objs, p)
	}
	return k8stest.NewFakeClient(objs...)
}
