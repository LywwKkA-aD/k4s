package dashboard_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// planLog is a refresh that changes nothing and records its plans, so the
// snapshot stays the one the dashboard built and the version row shows only
// what was handed over.
type planLog struct {
	mu    sync.Mutex
	plans []dash.Plan
}

func (l *planLog) fetch(_ context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.plans = append(l.plans, p)
	return prev
}

func (l *planLog) askedVersion() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.plans {
		for _, r := range p.Due {
			if r.Endpoint == dash.Version {
				return true
			}
		}
	}
	return false
}

func versionOptions(l *planLog, extra ...dashboard.Option) []dashboard.Option {
	clk := asynctest.NewClock(start)
	return append([]dashboard.Option{
		dashboard.WithFetch(l.fetch), dashboard.WithClock(clk.Now), dashboard.WithTicker(clk),
		dashboard.WithFrames(asynctest.NewClock(start)),
	}, extra...)
}

// TestTheRootsVersionIsTheDashboards: the root asks /version for the header
// and hands the answer over, so the dashboard never asks. An answer that
// lands while a refresh runs survives that refresh's reply.
func TestTheRootsVersionIsTheDashboards(t *testing.T) {
	t.Parallel()

	var l planLog
	m := dashboard.New(tuitest.NewFakeClient(t), nil, versionOptions(&l, dashboard.WithServerVersion(nil))...)
	m, _ = tuitest.Drive(t, m, tuitest.Resize(wideW, wideH))
	reply := tuitest.Collect(t, m.Init())
	m, _ = tuitest.Drive(t, m, dashboard.VersionMsg{Info: &apiversion.Info{GitVersion: "v1.36.5+k3s1", Minor: "36"}})
	m, _ = tuitest.Drive(t, m, reply...)
	if l.askedVersion() {
		t.Fatal("the dashboard planned /version the root asks")
	}
	if got := tuitest.Screen(m); !strings.Contains(got, "v1.36.5+k3s1") {
		t.Fatalf("the version the root handed over is not shown:\n%s", got)
	}
}

func TestAKnownVersionIsShownAtOnce(t *testing.T) {
	t.Parallel()

	var l planLog
	info := &apiversion.Info{GitVersion: "v1.31.4+k3s1", Minor: "31"}
	m := dashboard.New(tuitest.NewFakeClient(t), nil, versionOptions(&l, dashboard.WithServerVersion(info))...)
	m, _ = tuitest.Mount(t, m, wideW, wideH)
	if got := tuitest.Screen(m); l.askedVersion() || !strings.Contains(got, "v1.31.4+k3s1") {
		t.Fatalf("asked /version %v; screen:\n%s", l.askedVersion(), got)
	}
}

// TestWithoutTheRootsVersionTheDashboardAsks: a context the root does not
// ask /version for (a credential plugin started without a preflight)
// leaves the request to the dashboard.
func TestWithoutTheRootsVersionTheDashboardAsks(t *testing.T) {
	t.Parallel()

	var l planLog
	m := dashboard.New(tuitest.NewFakeClient(t), nil, versionOptions(&l)...)
	_, _ = tuitest.Mount(t, m, wideW, wideH)
	if !l.askedVersion() {
		t.Fatal("the dashboard did not ask /version with nobody else asking")
	}
}
