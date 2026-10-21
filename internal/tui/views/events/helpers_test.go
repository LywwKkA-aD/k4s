package events_test

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/events"
)

const (
	width  = 160
	height = 30

	demo = "demo"
)

var epoch = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// Event names of the fixture, newest first.
const (
	evBackOff  = "web-1.backoff"
	evPulled   = "web-1.pulled"
	evScaled   = "web.scaled"
	evNotReady = "node-1.notready"
	evApplied  = "traefik.applied"
)

func pod(ns, name string) corev1.ObjectReference {
	return corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: ns, Name: name}
}

// ev is an event in ns about obj, first seen first and last seen last
// before the epoch, the time the views of these tests measure from.
func ev(ns, name, typ, reason, msg string, obj corev1.ObjectReference, count int32, first, last time.Duration) *corev1.Event {
	return &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: ns, Name: name},
		InvolvedObject: obj,
		Type:           typ, Reason: reason, Message: msg, Count: count,
		FirstTimestamp: metav1.NewTime(epoch.Add(-first)),
		LastTimestamp:  metav1.NewTime(epoch.Add(-last)),
	}
}

// fixture is five events in three namespaces about a pod, a deployment,
// a node and a k3s HelmChart.
func fixture() []runtime.Object {
	return []runtime.Object{
		ev(demo, evBackOff, "Warning", "BackOff", "Back-off restarting failed container nginx", pod(demo, "web-1"), 5, 15*time.Hour, 10*time.Minute),
		ev(demo, evPulled, "Normal", "Pulled", "Container image \"nginx:1.27\" already present on machine", pod(demo, "web-1"), 1, 20*time.Minute, 20*time.Minute),
		ev(demo, evScaled, "Normal", "ScalingReplicaSet", "Scaled up replica set web-7d4b9c to 2",
			corev1.ObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Namespace: demo, Name: "web"}, 1, 30*time.Minute, 30*time.Minute),
		ev("default", evNotReady, "Warning", "NodeNotReady", "Node node-1 status is now: NodeNotReady",
			corev1.ObjectReference{APIVersion: "v1", Kind: "Node", Name: "node-1"}, 1, time.Hour, time.Hour),
		ev("kube-system", evApplied, "Normal", "ApplyJob", "Applying HelmChart using Job kube-system/helm-install-traefik",
			corev1.ObjectReference{APIVersion: "helm.cattle.io/v1", Kind: "HelmChart", Namespace: "kube-system", Name: "traefik"}, 1, 2*time.Hour, 2*time.Hour),
	}
}

func client(t *testing.T, opts ...tuitest.FakeOption) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, append([]tuitest.FakeOption{tuitest.WithObjects(fixture()...)}, opts...)...)
}

// clocked builds a view on a manual clock, so no test waits on a tick and
// LAST SEEN counts back from the clock's time.
func clocked(c *k8s.Client, namespace string) (events.Model, *asynctest.Clock) {
	clk := asynctest.NewClock(epoch)
	return events.New(c, namespace, events.WithClock(clk), events.WithNow(clk.Now)), clk
}

// mounted is a sized, loaded view on a manual clock, and what loading it
// emitted.
func mounted(t *testing.T, c *k8s.Client, namespace string) (events.Model, []tea.Msg) {
	t.Helper()
	m, _ := clocked(c, namespace)
	return tuitest.Mount(t, m, width, height)
}

// rows are the table lines under the header and its rule, trimmed.
func rows(m events.Model) []string {
	lines := screenLines(m)
	for i, l := range lines {
		if !hasHeader(l) {
			continue
		}
		out := lines[i+1:]
		if len(out) > 0 && strings.Trim(strings.TrimSpace(out[0]), "─") == "" {
			out = out[1:]
		}
		return out
	}
	return nil
}

func screenLines(m events.Model) []string {
	var out []string
	for _, l := range splitLines(tuitest.TrimLines(tuitest.Screen(m))) {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitLines(s string) []string { return strings.Split(s, "\n") }

func hasHeader(line string) bool { return strings.Contains(line, "LAST SEEN") }

func requireStatus(t *testing.T, msgs []tea.Msg, level status.Level, text string) {
	t.Helper()
	want := views.StatusMsg{Level: level, Text: text}
	if !slices.Contains(tuitest.All[views.StatusMsg](msgs), want) {
		t.Fatalf("want status %+v among:\n%s", want, tuitest.Describe(msgs))
	}
}

// helpLabel is the help text of the binding on k.
func helpLabel(t *testing.T, m events.Model, k string) string {
	t.Helper()
	for _, b := range m.Help() {
		if slices.Contains(b.Keys(), k) {
			return b.Help().Desc
		}
	}
	t.Fatalf("no %s binding", k)
	return ""
}

// rvLog records the ResourceVersion of every events list.
type rvLog struct {
	mu  sync.Mutex
	rvs []string
}

func (l *rvLog) reactor() tuitest.FakeOption {
	return tuitest.WithReactor("list", "events", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if la, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
			l.mu.Lock()
			l.rvs = append(l.rvs, la.GetListOptions().ResourceVersion)
			l.mu.Unlock()
		}
		return false, nil, nil
	})
}

func (l *rvLog) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.rvs)
}
