package k8s_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

const mi = 1024 * 1024

func metricsServer() http.Handler {
	pods := []k8stest.PodMetrics{
		{Namespace: "demo", Name: "web", Containers: []k8stest.ContainerUsage{
			{Name: "app", CPU: "250m", Memory: "64Mi"},
			{Name: "proxy", CPU: "5m", Memory: "16Mi"},
		}},
		{Namespace: "kube-system", Name: "coredns", Containers: []k8stest.ContainerUsage{
			{Name: "coredns", CPU: "0.003", Memory: "20Mi"},
		}},
		{Namespace: "demo", Name: "garbled", Containers: []k8stest.ContainerUsage{
			{Name: "app", CPU: "lots", Memory: "8Mi"},
			{Name: "side", CPU: "2m", Memory: "plenty"},
		}},
		{Namespace: "demo", Name: "starting"},
	}
	nodes := []k8stest.NodeMetrics{
		{Name: "server-0", CPU: "1250m", Memory: "2Gi"},
		{Name: "agent-0", CPU: "", Memory: "??"},
	}
	return k8stest.Metrics(pods, nodes)
}

func TestListPodMetricsOverREST(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		wantPath  string
		want      []k8s.PodMetric
	}{
		{
			name: "all namespaces", namespace: "", wantPath: "/apis/metrics.k8s.io/v1beta1/pods",
			want: []k8s.PodMetric{
				{Name: "web", Namespace: "demo", CPUMilli: 255, MemBytes: 80 * mi},
				{Name: "coredns", Namespace: "kube-system", CPUMilli: 3, MemBytes: 20 * mi},
				// Unparseable quantities count as zero; the rest of the pod survives.
				{Name: "garbled", Namespace: "demo", CPUMilli: 2, MemBytes: 8 * mi},
				{Name: "starting", Namespace: "demo"},
			},
		},
		{
			name: "one namespace", namespace: "kube-system", wantPath: "/apis/metrics.k8s.io/v1beta1/namespaces/kube-system/pods",
			want: []k8s.PodMetric{{Name: "coredns", Namespace: "kube-system", CPUMilli: 3, MemBytes: 20 * mi}},
		},
		{
			name: "namespace without pods", namespace: "empty", wantPath: "/apis/metrics.k8s.io/v1beta1/namespaces/empty/pods",
			want: []k8s.PodMetric{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &pathRecorder{next: metricsServer()}
			c := k8stest.NewServerClient(t, rec)

			got, err := c.ListPodMetrics(context.Background(), tc.namespace)
			if err != nil {
				t.Fatalf("ListPodMetrics: %v", err)
			}
			if got == nil || !slices.Equal(got, tc.want) {
				t.Fatalf("ListPodMetrics = %+v, want %+v", got, tc.want)
			}
			if paths := rec.seen(); !slices.Equal(paths, []string{tc.wantPath}) {
				t.Fatalf("requested %v, want [%s]", paths, tc.wantPath)
			}
		})
	}
}

func TestListNodeMetricsOverREST(t *testing.T) {
	t.Parallel()

	rec := &pathRecorder{next: metricsServer()}
	c := k8stest.NewServerClient(t, rec)

	got, err := c.ListNodeMetrics(context.Background())
	if err != nil {
		t.Fatalf("ListNodeMetrics: %v", err)
	}
	want := []k8s.NodeMetric{
		{Name: "server-0", CPUMilli: 1250, MemBytes: 2 * 1024 * mi},
		{Name: "agent-0"}, // empty and unparseable quantities count as zero
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ListNodeMetrics = %+v, want %+v", got, want)
	}
	if paths := rec.seen(); !slices.Equal(paths, []string{"/apis/metrics.k8s.io/v1beta1/nodes"}) {
		t.Fatalf("requested %v, want the nodes endpoint once", paths)
	}
}

// metricsCall runs one metrics query and returns only its error.
type metricsCall struct {
	name   string
	decode string // prefix of the error when the body is not JSON
	prefix string // the CallError the failure is wrapped in
	call   func(context.Context, *k8s.Client) error
}

func metricsCalls() []metricsCall {
	return []metricsCall{
		{name: "pods", decode: "decode pod metrics: ", prefix: "list pods.metrics.k8s.io: ", call: func(ctx context.Context, c *k8s.Client) error {
			_, err := c.ListPodMetrics(ctx, "")
			return err
		}},
		{name: "pods in namespace", decode: "decode pod metrics: ", prefix: "list pods.metrics.k8s.io: ", call: func(ctx context.Context, c *k8s.Client) error {
			_, err := c.ListPodMetrics(ctx, "demo")
			return err
		}},
		{name: "nodes", decode: "decode node metrics: ", prefix: "list nodes.metrics.k8s.io: ", call: func(ctx context.Context, c *k8s.Client) error {
			_, err := c.ListNodeMetrics(ctx)
			return err
		}},
	}
}

// TestMetricsErrorsOverREST: a cluster without a serving metrics API (404,
// 503) is ErrMetricsUnavailable; RBAC denying metrics is a Forbidden that
// keeps the apiserver's wording, so the classifier can name the can-i.
func TestMetricsErrorsOverREST(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		handler     http.Handler
		is          func(error) bool
		unavailable bool
		wantMsg     string
	}{
		{
			name: "metrics-server not installed (404)", handler: k8stest.NotFound(), is: apierrors.IsNotFound, unavailable: true,
			wantMsg: "metrics API unavailable: the server could not find the requested resource",
		},
		{
			name: "metrics-server not ready (503)", handler: k8stest.ServiceUnavailable(), is: apierrors.IsServiceUnavailable, unavailable: true,
			wantMsg: "metrics API unavailable: the server is currently unable to handle the request",
		},
		{
			name: "RBAC denies metrics (403)", handler: k8stest.Forbidden(), is: apierrors.IsForbidden,
			wantMsg: "is forbidden: User \"" + k8stest.User + "\" cannot list resource",
		},
	}
	for _, tc := range cases {
		for _, mc := range metricsCalls() {
			t.Run(tc.name+"/"+mc.name, func(t *testing.T) {
				t.Parallel()
				c := k8stest.NewServerClient(t, tc.handler)

				err := mc.call(context.Background(), c)
				if !tc.is(err) {
					t.Fatalf("err = %v (reason %q), want it classified by %s", err, apierrors.ReasonForError(err), tc.name)
				}
				if got := errors.Is(err, k8s.ErrMetricsUnavailable); got != tc.unavailable {
					t.Fatalf("errors.Is(%v, ErrMetricsUnavailable) = %v, want %v", err, got, tc.unavailable)
				}
				if !strings.HasPrefix(err.Error(), mc.prefix) || !strings.Contains(err.Error(), tc.wantMsg) {
					t.Fatalf("err = %q, want %q then %q", err, mc.prefix, tc.wantMsg)
				}
			})
		}
	}
}

// TestMetricsForbiddenNamesTheCanI: the 403 classifies with the scope the
// view asked for.
func TestMetricsForbiddenNamesTheCanI(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"pods":              "kubectl auth can-i list pods.metrics.k8s.io -A",
		"pods in namespace": "kubectl auth can-i list pods.metrics.k8s.io -n demo",
		"nodes":             "kubectl auth can-i list nodes.metrics.k8s.io",
	}
	for _, mc := range metricsCalls() {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Forbidden())
			d := k8s.Classify(mc.call(context.Background(), c))
			if d.Kind != k8s.KindForbidden || d.Hint != want[mc.name] {
				t.Fatalf("Classify = %v %q, want forbidden with %q", d.Kind, d.Hint, want[mc.name])
			}
		})
	}
}

// TestMetricsWithoutDiscovery: a client whose discovery has no REST client
// (a fake clientset) reports metrics as unavailable instead of panicking.
func TestMetricsWithoutDiscovery(t *testing.T) {
	t.Parallel()

	for _, mc := range metricsCalls() {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient()
			if err := mc.call(context.Background(), c); !errors.Is(err, k8s.ErrMetricsUnavailable) {
				t.Fatalf("err = %v, want ErrMetricsUnavailable", err)
			}
		})
	}
}

// TestPodMetricsRejectsABadNamespace: the namespace is a path segment, so a
// name that is not one never reaches the server.
func TestPodMetricsRejectsABadNamespace(t *testing.T) {
	t.Parallel()

	rec := &pathRecorder{next: metricsServer()}
	c := k8stest.NewServerClient(t, rec)
	_, err := c.ListPodMetrics(context.Background(), "../nodes")
	if !errors.Is(err, k8s.ErrInvalidNamespace) {
		t.Fatalf("err = %v, want ErrInvalidNamespace", err)
	}
	if paths := rec.seen(); len(paths) != 0 {
		t.Fatalf("requested %v for an invalid namespace", paths)
	}
}

func TestMetricsHonourTheCallerDeadline(t *testing.T) {
	t.Parallel()

	for _, mc := range metricsCalls() {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Hang())
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := mc.call(ctx, c)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want context.DeadlineExceeded", err)
			}
			if !strings.HasPrefix(err.Error(), mc.prefix) {
				t.Fatalf("err = %q, want prefix %q", err, mc.prefix)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("metrics call outlived its deadline: %v", elapsed)
			}
		})
	}
}

func TestMetricsRejectsANonJSONBody(t *testing.T) {
	t.Parallel()

	for _, mc := range metricsCalls() {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html>captive portal</html>"))
			}))

			err := mc.call(context.Background(), c)
			if err == nil || !strings.HasPrefix(err.Error(), mc.decode) {
				t.Fatalf("err = %v, want prefix %q", err, mc.decode)
			}
		})
	}
}
