package k8stest_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func TestNewFakeClient(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "demo"}}
	c, cs := k8stest.NewFakeClient(pod)
	if c.Clientset != cs {
		t.Fatal("client does not wrap the returned clientset")
	}
	if c.Context != k8stest.ContextName {
		t.Fatalf("Context = %q, want %q", c.Context, k8stest.ContextName)
	}
	k8stest.Unauthorized(cs)
	if _, err := c.ListPods(context.Background(), "demo"); err == nil {
		t.Fatal("reactor installed on the returned clientset did not reach the client")
	}
}

// TestNewFakeClientHasNoMetricsAPI: the fake clientset has no REST client
// behind Discovery(), so a metrics query says the API is unavailable.
func TestNewFakeClientHasNoMetricsAPI(t *testing.T) {
	t.Parallel()

	c, _ := k8stest.NewFakeClient()
	if _, err := c.ListPodMetrics(context.Background(), ""); !errors.Is(err, k8s.ErrMetricsUnavailable) {
		t.Errorf("ListPodMetrics = %v, want ErrMetricsUnavailable", err)
	}
	if _, err := c.ListNodeMetrics(context.Background()); !errors.Is(err, k8s.ErrMetricsUnavailable) {
		t.Errorf("ListNodeMetrics = %v, want ErrMetricsUnavailable", err)
	}
}

func TestNewServerClientTalksToTheHandler(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	c := k8stest.NewServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/namespaces/demo/pods" {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","metadata":{},"items":[{"metadata":{"name":"web","namespace":"demo"}}]}`))
	}))
	if c.RestConfig == nil || c.RestConfig.Host == "" {
		t.Fatal("RestConfig.Host not set to the test server")
	}
	if c.Context != k8stest.ContextName {
		t.Fatalf("Context = %q, want %q", c.Context, k8stest.ContextName)
	}

	pods, err := c.ListPods(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "web" || hits.Load() != 1 {
		t.Fatalf("pods = %+v, hits = %d, want web from one request", pods, hits.Load())
	}
}

func TestNewServerClientHonoursTheCallerDeadline(t *testing.T) {
	t.Parallel()

	c := k8stest.NewServerClient(t, k8stest.Hang())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.ListPods(ctx, "demo")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("request outlived its deadline: %v", elapsed)
	}
}
