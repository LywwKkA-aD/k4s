package top_test

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
)

// changing is a metrics server whose answer the test swaps between polls.
type changing struct {
	h atomic.Pointer[http.Handler]
}

func (c *changing) set(h http.Handler) { c.h.Store(&h) }

func (c *changing) ServeHTTP(w http.ResponseWriter, r *http.Request) { (*c.h.Load()).ServeHTTP(w, r) }

func pod(name, cpu string) k8stest.PodMetrics {
	return k8stest.PodMetrics{Namespace: "demo", Name: name, Containers: []k8stest.ContainerUsage{{Name: "app", CPU: cpu, Memory: "1Mi"}}}
}

func serving(pods ...k8stest.PodMetrics) http.Handler {
	return k8stest.Metrics(pods, nil)
}

// onRow is the row key the cursor is on, as State reports it.
func onRow(m top.Model) string { return m.State().Selected }

// TestCursorFollowsItsRowAcrossSorts: s re-sorts, and the cursor stays on
// the pod it was on instead of on the same index.
func TestCursorFollowsItsRowAcrossSorts(t *testing.T) {
	t.Parallel()

	m := mounted(t, skewed(t), "demo")
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // mem-hog under cpu-hog by cpu
	if got := onRow(m); got != "demo/mem-hog" {
		t.Fatalf("cursor on %q, want demo/mem-hog", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("s")) // by memory: mem-hog is first now
	if got := onRow(m); got != "demo/mem-hog" {
		t.Errorf("after s the cursor is on %q, want demo/mem-hog", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("s"))
	if got := onRow(m); got != "demo/mem-hog" {
		t.Errorf("after s again the cursor is on %q, want demo/mem-hog", got)
	}
}

// TestCursorFollowsItsRowAcrossPolls: usage moves between polls, rows
// change places, and the cursor goes with its pod.
func TestCursorFollowsItsRowAcrossPolls(t *testing.T) {
	t.Parallel()

	var srv changing
	srv.set(serving(pod("a", "300m"), pod("b", "200m"), pod("c", "100m")))
	m, clk := clocked(t, k8stest.NewServerClient(t, &srv), "demo")
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // b
	if got := onRow(m); got != "demo/b" {
		t.Fatalf("cursor on %q, want demo/b", got)
	}

	srv.set(serving(pod("a", "100m"), pod("b", "200m"), pod("c", "300m"), pod("d", "400m")))
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := onRow(m); got != "demo/b" {
		t.Errorf("after the pods swapped places the cursor is on %q, want demo/b", got)
	}
	if m.State().Cursor != 2 {
		t.Errorf("cursor index = %d, want 2 (d, c, b)", m.State().Cursor)
	}
}

// TestCursorFallsBackToTheClampedIndex: when its pod is gone the cursor
// keeps its index, clamped to the rows that are left.
func TestCursorFallsBackToTheClampedIndex(t *testing.T) {
	t.Parallel()

	var srv changing
	srv.set(serving(pod("a", "300m"), pod("b", "200m"), pod("c", "100m")))
	m, clk := clocked(t, k8stest.NewServerClient(t, &srv), "demo")
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"), tuitest.Press("down")) // c, the last

	srv.set(serving(pod("a", "300m"), pod("b", "200m")))
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := onRow(m); got != "demo/b" {
		t.Errorf("with c gone the cursor is on %q, want the last row, demo/b", got)
	}

	srv.set(serving(pod("a", "300m"), pod("x", "250m"), pod("b", "200m")))
	tick, _ = clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := onRow(m); got != "demo/b" {
		t.Errorf("b moved down and the cursor lost it: on %q", got)
	}
}
