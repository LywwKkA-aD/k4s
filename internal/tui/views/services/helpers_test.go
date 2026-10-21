package services_test

import (
	"context"
	"slices"
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
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

const (
	width  = 160
	height = 30

	// shop is the namespace of the "pay" fixture.
	shop = "shop"
)

var epoch = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// clocked builds a view on a manual clock, so no test waits on a tick or
// on the logs notice delay.
func clocked(c *k8s.Client, namespace string) (services.Model, *asynctest.Clock) {
	clk := asynctest.NewClock(epoch)
	return services.New(c, namespace, services.WithClock(clk)), clk
}

// mounted is a sized, loaded view on a manual clock.
func mounted(t *testing.T, c *k8s.Client, namespace string) services.Model {
	t.Helper()
	m, _ := clocked(c, namespace)
	m, _ = tuitest.Mount(t, m, width, height)
	return m
}

func on(name string, more ...tea.Msg) []tea.Msg {
	return slices.Concat(tuitest.FilterBy(name), more)
}

// payServices is the "pay" fixture in shop, sorted as the table lists
// it: api, pay-api, pay-db, pay-web, payments, web. Each service selects
// one running pod of its own name with one nginx container.
func payServices() []runtime.Object {
	names := []string{"api", "pay-api", "pay-db", "pay-web", "payments", "web"}
	objs := make([]runtime.Object, 0, 1+2*len(names))
	objs = append(objs, tuitest.Namespace(shop))
	for i, n := range names {
		labels := map[string]string{"app": n}
		objs = append(objs,
			tuitest.Service(shop, n, int32(8000+i), labels),
			tuitest.Pod(shop, n+"-1", labels, tuitest.ContainerNginx),
		)
	}
	return objs
}

// rvLog records the ResourceVersion of every services list.
type rvLog struct {
	mu  sync.Mutex
	rvs []string
}

func (l *rvLog) reactor() tuitest.FakeOption {
	return tuitest.WithReactor("list", "services", func(a k8stesting.Action) (bool, runtime.Object, error) {
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

// statuses returns the status messages among msgs.
func statuses(msgs []tea.Msg) []views.StatusMsg {
	return tuitest.All[views.StatusMsg](msgs)
}

func requireStatus(t *testing.T, msgs []tea.Msg, level status.Level, text string) {
	t.Helper()
	want := views.StatusMsg{Level: level, Text: text}
	if !slices.Contains(statuses(msgs), want) {
		t.Fatalf("want status %+v among:\n%s", want, tuitest.Describe(msgs))
	}
}

// press feeds keys through Update one at a time and returns the Cmd of
// the last one without running it, so a test can hold a reply back.
func press(t *testing.T, m services.Model, keys ...string) (services.Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next views.View
		next, cmd = m.Update(tuitest.Press(k))
		typed, ok := next.(services.Model)
		if !ok {
			t.Fatalf("Update returned %T", next)
		}
		m = typed
	}
	return m, cmd
}

func deleteService(t *testing.T, c *k8s.Client, namespace, name string) {
	t.Helper()
	if err := c.Clientset.CoreV1().Services(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete service %s/%s: %v", namespace, name, err)
	}
}

func createService(t *testing.T, c *k8s.Client, svc *corev1.Service) {
	t.Helper()
	if _, err := c.Clientset.CoreV1().Services(svc.Namespace).Create(context.Background(), svc, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create service %s/%s: %v", svc.Namespace, svc.Name, err)
	}
}

func payClient(t *testing.T, opts ...tuitest.FakeOption) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, append([]tuitest.FakeOption{tuitest.WithObjects(payServices()...)}, opts...)...)
}

// describes reports which row Enter describes.
func describes(t *testing.T, m services.Model) string {
	t.Helper()
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	got, ok := tuitest.Find[views.DescribeRequestMsg](out)
	if !ok {
		t.Fatalf("enter described nothing:\n%s", tuitest.Describe(out))
	}
	return views.RowKey(got.Namespace, got.Name)
}
