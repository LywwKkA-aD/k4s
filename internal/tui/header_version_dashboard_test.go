package tui

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	versionDenied = "version forbidden"
	// dashW and dashH are a terminal the dashboard draws its cluster panel
	// in, as the root tests' screen.
	dashW, dashH = 160, 40
)

func deniedVersion() error {
	return apierrors.NewForbidden(versionResource, "", errors.New("rbac"))
}

// versionServer answers /version with codes, one per request, the last
// repeating, then with v1.36.5+k3s1; every other path is a 404. asks
// counts the /version requests.
func versionServer(t *testing.T, asks *atomic.Int32, codes ...int) http.Handler {
	t.Helper()
	notFound := k8stest.NotFound()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			notFound.ServeHTTP(w, r)
			return
		}
		n := int(asks.Add(1))
		if n <= len(codes) {
			switch codes[n-1] {
			case http.StatusForbidden:
				k8stest.Forbidden().ServeHTTP(w, r)
			default:
				http.Error(w, http.StatusText(codes[n-1]), codes[n-1])
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.5+k3s1"}`))
	})
}

func screenHas(m Model, want string) bool { return strings.Contains(tuitest.Screen(m), want) }

// TestDeniedVersionShowsOnTheDashboard: RBAC refused the header's
// /version. The dashboard, which leaves /version to the header, says so on
// its version row with the can-i probe, as it says any denial, and asks
// nothing more.
func TestDeniedVersionShowsOnTheDashboard(t *testing.T) {
	t.Parallel()

	var asks atomic.Int32
	c := k8stest.NewServerClient(t, versionServer(t, &asks, http.StatusForbidden))
	m, _ := tuitest.Mount(t, New(c, nil), dashW, dashH)
	if !screenHas(m, versionDenied) {
		t.Fatalf("the denied /version is silent:\n%s", tuitest.Screen(m))
	}
	if n := asks.Load(); n != 1 {
		t.Fatalf("/version asked %d times, want 1", n)
	}
}

// TestVersionDownThenUpReachesTheDashboard: a server not ready at the start
// is no denial; the header's retry gets the version and the dashboard shows
// it.
func TestVersionDownThenUpReachesTheDashboard(t *testing.T) {
	t.Parallel()

	var asks atomic.Int32
	c := k8stest.NewServerClient(t, versionServer(t, &asks, http.StatusServiceUnavailable))
	m, _ := tuitest.Mount(t, New(c, nil), dashW, dashH)
	if screenHas(m, "forbidden") || screenHas(m, "v1.36.5+k3s1") {
		t.Fatalf("a server not ready reads as a denial or a version:\n%s", tuitest.Screen(m))
	}
	m, _ = tuitest.Drive(t, m, versionRetryMsg{token: m.ident.version, attempt: 1})
	if !screenHas(m, "v1.36.5+k3s1") {
		t.Fatalf("the retry's version is not on the dashboard:\n%s", tuitest.Screen(m))
	}
}

// TestDeniedVersionReachesALaterDashboard: the denial lands while another
// view is on screen; the dashboard built when the user comes back shows it.
func TestDeniedVersionReachesALaterDashboard(t *testing.T) {
	t.Parallel()

	m := New(tuitest.NewFakeClient(t), nil)
	m, _ = tuitest.Drive(t, m, tuitest.Resize(dashW, dashH))
	m, _ = tuitest.Drive(t, m, tuitest.Colon("pods")...)
	m, _ = tuitest.Drive(t, m, versionReply(m.ident.version, 0, nil, deniedVersion()))
	m, _ = tuitest.Drive(t, m, tuitest.Press("q"))
	if m.current.Title() != "dashboard" || !screenHas(m, versionDenied) {
		t.Fatalf("the dashboard built after the denial does not show it:\n%s", tuitest.Screen(m))
	}
}

// TestContextSwitchDropsTheDeniedVersion: :ctx leaves the old context's
// denial behind, and a late reply for the old client changes nothing.
func TestContextSwitchDropsTheDeniedVersion(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, New(tuitest.NewFakeClient(t), nil), dashW, dashH)
	old := m.ident.version
	m, _ = tuitest.Drive(t, m, versionReply(old, 0, nil, deniedVersion()))
	if !screenHas(m, versionDenied) {
		t.Fatalf("the denial is not shown before :ctx:\n%s", tuitest.Screen(m))
	}
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: testContextBeta})
	if screenHas(m, versionDenied) {
		t.Fatalf("the old context's denial survived :ctx:\n%s", tuitest.Screen(m))
	}
	m, _ = tuitest.Drive(t, m, versionReply(old, 0, nil, deniedVersion()))
	if screenHas(m, versionDenied) {
		t.Fatalf("a late denial for the old client reached the new dashboard:\n%s", tuitest.Screen(m))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Colon("pods")...)
	m, _ = tuitest.Drive(t, m, tuitest.Press("q"))
	if screenHas(m, versionDenied) {
		t.Fatalf("a dashboard built after :ctx shows the old denial:\n%s", tuitest.Screen(m))
	}
}
