package tui_test

import (
	"net/http"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestDashboardShowsTheHeadersVersion: a context costs one /version. The
// header asks it and the dashboard's cluster panel shows the same answer
// instead of asking again.
func TestDashboardShowsTheHeadersVersion(t *testing.T) {
	t.Parallel()

	var versions atomic.Int32
	notFound := k8stest.NotFound()
	c := k8stest.NewServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			notFound.ServeHTTP(w, r)
			return
		}
		versions.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.5+k3s1"}`))
	}))
	m, _ := tuitest.Mount(t, tui.New(c, nil), screenW, screenH)
	requireScreen(t, m, "v1.36.5+k3s1")
	if got := versions.Load(); got != 1 {
		t.Fatalf("/version asked %d times, want 1", got)
	}
}

// TestDashboardHintsFollowTheSessionsDistro: the distribution the root
// detected words the dashboard's metrics hint, here minikube's addon,
// which the server version alone does not tell apart.
func TestDashboardHintsFollowTheSessionsDistro(t *testing.T) {
	t.Parallel()

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "minikube"}}
	c := tuitest.NewFakeClient(t, tuitest.WithContext("minikube"), tuitest.WithObjects(node), tuitest.WithMetricsStatus(http.StatusNotFound))
	m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(startup.Session{Distro: cluster.Minikube})), 207, 58)
	requireScreen(t, m, cluster.MetricsHint(cluster.Minikube))
}
