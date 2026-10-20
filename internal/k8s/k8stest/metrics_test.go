package k8stest_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// wireList mirrors the metrics.k8s.io/v1beta1 list envelope closely enough
// to prove the handler emits what metrics-server would.
type wireList struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Items      []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Timestamp  string `json:"timestamp"`
		Window     string `json:"window"`
		Containers []struct {
			Name  string            `json:"name"`
			Usage map[string]string `json:"usage"`
		} `json:"containers"`
		Usage map[string]string `json:"usage"`
	} `json:"items"`
}

func metricsFixture() http.Handler {
	pods := []k8stest.PodMetrics{
		{Namespace: "demo", Name: "web", Containers: []k8stest.ContainerUsage{
			{Name: "app", CPU: "250m", Memory: "64Mi"},
			{Name: "proxy", CPU: "5m", Memory: "16Mi"},
		}},
		{Namespace: "kube-system", Name: "coredns", Containers: []k8stest.ContainerUsage{
			{Name: "coredns", CPU: "3m", Memory: "20Mi"},
		}},
	}
	nodes := []k8stest.NodeMetrics{{Name: "server-0", CPU: "1250m", Memory: "2Gi"}}
	return k8stest.Metrics(pods, nodes)
}

func decodeList(t *testing.T, body []byte) wireList {
	t.Helper()
	var l wireList
	if err := json.Unmarshal(body, &l); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return l
}

func TestMetricsPodLists(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		target    string
		wantNames []string
	}{
		{name: "all namespaces", target: "/apis/metrics.k8s.io/v1beta1/pods", wantNames: []string{"web", "coredns"}},
		{name: "one namespace", target: "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods", wantNames: []string{"web"}},
		{name: "empty namespace", target: "/apis/metrics.k8s.io/v1beta1/namespaces/empty/pods", wantNames: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := serve(t, metricsFixture(), http.MethodGet, tc.target)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("code = %d, Content-Type = %q, want 200 JSON", rec.Code, rec.Header().Get("Content-Type"))
			}
			l := decodeList(t, rec.Body.Bytes())
			if l.Kind != "PodMetricsList" || l.APIVersion != "metrics.k8s.io/v1beta1" {
				t.Fatalf("envelope = %s %s, want PodMetricsList metrics.k8s.io/v1beta1", l.Kind, l.APIVersion)
			}
			if l.Items == nil {
				t.Fatal("items is null, metrics-server sends []")
			}
			got := make([]string, 0, len(l.Items))
			for _, it := range l.Items {
				got = append(got, it.Metadata.Name)
				if it.Timestamp == "" || it.Window == "" {
					t.Errorf("item %s lacks timestamp/window", it.Metadata.Name)
				}
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("items = %v, want %v", got, tc.wantNames)
			}
			for i := range got {
				if got[i] != tc.wantNames[i] {
					t.Fatalf("items = %v, want %v", got, tc.wantNames)
				}
			}
		})
	}
}

func TestMetricsPodUsageIsVerbatim(t *testing.T) {
	t.Parallel()

	rec := serve(t, metricsFixture(), http.MethodGet, "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods")
	l := decodeList(t, rec.Body.Bytes())
	if len(l.Items) != 1 || len(l.Items[0].Containers) != 2 {
		t.Fatalf("items = %+v, want web with two containers", l.Items)
	}
	app := l.Items[0].Containers[0]
	if app.Name != "app" || app.Usage["cpu"] != "250m" || app.Usage["memory"] != "64Mi" {
		t.Fatalf("container = %+v, want app 250m/64Mi", app)
	}
	if l.Items[0].Metadata.Namespace != "demo" {
		t.Fatalf("namespace = %q, want demo", l.Items[0].Metadata.Namespace)
	}
}

func TestMetricsNodeList(t *testing.T) {
	t.Parallel()

	rec := serve(t, metricsFixture(), http.MethodGet, "/apis/metrics.k8s.io/v1beta1/nodes")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	l := decodeList(t, rec.Body.Bytes())
	if l.Kind != "NodeMetricsList" || len(l.Items) != 1 {
		t.Fatalf("list = %+v, want one NodeMetricsList item", l)
	}
	n := l.Items[0]
	if n.Metadata.Name != "server-0" || n.Usage["cpu"] != "1250m" || n.Usage["memory"] != "2Gi" {
		t.Fatalf("node = %+v, want server-0 1250m/2Gi", n)
	}
}

func TestMetricsUnknownPathIsNotFound(t *testing.T) {
	t.Parallel()

	rec := serve(t, metricsFixture(), http.MethodGet, "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods/web/extra")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
	decodeStatus(t, rec)
}

func TestMetricsCopiesItsInput(t *testing.T) {
	t.Parallel()

	pods := []k8stest.PodMetrics{{Namespace: "demo", Name: "web", Containers: []k8stest.ContainerUsage{{Name: "app", CPU: "1m", Memory: "1Mi"}}}}
	nodes := []k8stest.NodeMetrics{{Name: "n", CPU: "1m", Memory: "1Mi"}}
	h := k8stest.Metrics(pods, nodes)
	pods[0].Containers[0].CPU = "999m"
	nodes[0].Name = "changed"

	podList := decodeList(t, serve(t, h, http.MethodGet, "/apis/metrics.k8s.io/v1beta1/pods").Body.Bytes())
	if got := podList.Items[0].Containers[0].Usage["cpu"]; got != "1m" {
		t.Fatalf("cpu = %q after the caller mutated its slice, want 1m", got)
	}
	nodeList := decodeList(t, serve(t, h, http.MethodGet, "/apis/metrics.k8s.io/v1beta1/nodes").Body.Bytes())
	if got := nodeList.Items[0].Metadata.Name; got != "n" {
		t.Fatalf("node = %q after the caller mutated its slice, want n", got)
	}
}
