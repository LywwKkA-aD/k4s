package pods_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

const (
	width  = 140
	height = 30
)

var epoch = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// newView builds a pods view whose ticks run on a manual clock, so no test
// waits on the 5s refresh.
func newView(c *k8s.Client, namespace string) pods.Model {
	return pods.New(c, namespace, pods.WithClock(asynctest.NewClock(epoch)))
}

// mountedAt sizes and starts a view on clk: its first list has landed and
// the next tick is armed.
func mountedAt(t *testing.T, c *k8s.Client, namespace string, clk *asynctest.Clock) pods.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, pods.New(c, namespace, pods.WithClock(clk)), width, height)
	return m
}

func mounted(t *testing.T, c *k8s.Client, namespace string) pods.Model {
	t.Helper()
	return mountedAt(t, c, namespace, asynctest.NewClock(epoch))
}

func keys(prefix []tea.Msg, more ...tea.Msg) []tea.Msg {
	return slices.Concat(prefix, more)
}

func podRef(ns, name string) views.ResourceRef {
	return views.ResourceRef{Kind: "pod", Namespace: ns, Name: name}
}

// rvLog records the ResourceVersion of every pod list the fake API serves.
type rvLog struct {
	mu  sync.Mutex
	rvs []string
}

type listOptionsAction interface {
	GetListOptions() metav1.ListOptions
}

func (l *rvLog) option() tuitest.FakeOption {
	return tuitest.WithReactor("list", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if la, ok := a.(listOptionsAction); ok {
			l.mu.Lock()
			l.rvs = append(l.rvs, la.GetListOptions().ResourceVersion)
			l.mu.Unlock()
		}
		return false, nil, nil
	})
}

func (l *rvLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.rvs)
}

// watchHint is the help text of the view's w binding.
func watchHint(m pods.Model) string {
	for _, b := range m.Help() {
		if slices.Contains(b.Keys(), "w") {
			return b.Help().Desc
		}
	}
	return ""
}

// selected is the row key under the cursor, as State reports it.
func selected(m pods.Model) string {
	return m.State().Selected
}

// down moves the cursor n rows.
func down(n int) []tea.Msg {
	out := make([]tea.Msg, 0, n)
	for range n {
		out = append(out, tuitest.Press("down"))
	}
	return out
}
