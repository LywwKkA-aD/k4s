package dashboard_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

var versionResource = schema.GroupResource{Resource: "version"}

// TestARootsFailedVersionIsTheDashboards: the root's /version request
// failed, so the dashboard, which does not ask it, says so as it says a
// failure of its own requests: a denial shows — with its can-i probe, a
// server away leaves the row out. A refresh that started before the
// failure landed does not lose it, and the root's later answer replaces it.
func TestARootsFailedVersionIsTheDashboards(t *testing.T) {
	t.Parallel()

	answer := dashboard.VersionMsg{Info: &apiversion.Info{GitVersion: "v1.36.5+k3s1", Minor: "36"}}
	cases := []struct {
		name   string
		err    error
		failed string // on screen after the failure
	}{
		{name: "denied", err: apierrors.NewForbidden(versionResource, "", errors.New("rbac")), failed: "version forbidden"},
		{name: "server down", err: &url.Error{Op: "Get", URL: "https://10.0.0.5:6443/version", Err: syscall.ECONNREFUSED}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var l planLog
			m := dashboard.New(tuitest.NewFakeClient(t), nil, versionOptions(&l, dashboard.WithServerVersion(nil))...)
			m, _ = tuitest.Drive(t, m, tuitest.Resize(wideW, wideH))
			reply := tuitest.Collect(t, m.Init())
			m, _ = tuitest.Drive(t, m, dashboard.VersionMsg{Err: tc.err})
			m, _ = tuitest.Drive(t, m, reply...)
			got := tuitest.Screen(m)
			if tc.failed != "" && !strings.Contains(got, tc.failed) {
				t.Fatalf("the root's failure is not shown as %q:\n%s", tc.failed, got)
			}
			if tc.failed == "" && strings.Contains(got, "forbidden") {
				t.Fatalf("a server away reads as a denial:\n%s", got)
			}
			if l.askedVersion() {
				t.Fatal("the dashboard planned /version after the root's failed")
			}
			m, _ = tuitest.Drive(t, m, answer)
			if got := tuitest.Screen(m); !strings.Contains(got, "v1.36.5+k3s1") || strings.Contains(got, "version forbidden") {
				t.Fatalf("the root's answer did not replace its failure:\n%s", got)
			}
		})
	}
}

// TestAKnownFailureIsShownAtOnce: a dashboard built after the root's
// /version request failed shows the failure from its first frame.
func TestAKnownFailureIsShownAtOnce(t *testing.T) {
	t.Parallel()

	var l planLog
	denied := apierrors.NewForbidden(versionResource, "", errors.New("rbac"))
	m := dashboard.New(tuitest.NewFakeClient(t), nil, versionOptions(&l, dashboard.WithServerVersion(nil), dashboard.WithServerVersionError(denied))...)
	m, _ = tuitest.Mount(t, m, wideW, wideH)
	if got := tuitest.Screen(m); l.askedVersion() || !strings.Contains(got, "version forbidden") {
		t.Fatalf("asked /version %v; screen:\n%s", l.askedVersion(), got)
	}
}
