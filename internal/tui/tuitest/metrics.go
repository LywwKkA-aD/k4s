package tuitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// NodeName is the one node the fake metrics API reports.
const NodeName = "node-1"

const metricsPrefix = "/apis/metrics.k8s.io/v1beta1/"

type usage struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type meta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type containerMetrics struct {
	Usage usage `json:"usage"`
}

type podMetrics struct {
	Metadata   meta               `json:"metadata"`
	Containers []containerMetrics `json:"containers"`
}

type nodeMetrics struct {
	Metadata meta  `json:"metadata"`
	Usage    usage `json:"usage"`
}

// demoPodMetrics matches the demo seed. multi-0 is the busiest pod, so the
// top view (sorted by CPU) lists it first.
func demoPodMetrics() []podMetrics {
	one := func(ns, name, cpu, mem string) podMetrics {
		return podMetrics{Metadata: meta{Name: name, Namespace: ns}, Containers: []containerMetrics{{Usage: usage{CPU: cpu, Memory: mem}}}}
	}
	return []podMetrics{
		one(NamespaceDemo, PodWeb1, "12m", "20Mi"),
		one(NamespaceDemo, PodWeb2, "8m", "18Mi"),
		{
			Metadata: meta{Name: PodMulti, Namespace: NamespaceDemo},
			Containers: []containerMetrics{
				{Usage: usage{CPU: "20m", Memory: "40Mi"}},
				{Usage: usage{CPU: "10m", Memory: "24Mi"}},
			},
		},
		one(NamespaceDemo, PodCrash, "0", "1Mi"),
		one(NamespaceSystem, PodCoreDNS, "3m", "15Mi"),
	}
}

// metricsRoundTripper serves metrics.k8s.io from memory, or status for
// every request when it is not 200.
type metricsRoundTripper struct {
	status int
}

func (rt metricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.status != http.StatusOK {
		return respond(req, rt.status, nil), nil
	}
	body, ok, err := metricsBody(strings.TrimPrefix(req.URL.Path, metricsPrefix))
	if err != nil {
		return nil, err
	}
	if !ok {
		return respond(req, http.StatusNotFound, nil), nil
	}
	return respond(req, http.StatusOK, body), nil
}

// metricsBody answers "pods", "namespaces/<ns>/pods" and "nodes".
func metricsBody(path string) ([]byte, bool, error) {
	var items any
	switch {
	case path == "nodes":
		items = []nodeMetrics{{Metadata: meta{Name: NodeName}, Usage: usage{CPU: "250m", Memory: "1Gi"}}}
	case path == "pods":
		items = demoPodMetrics()
	case strings.HasPrefix(path, "namespaces/") && strings.HasSuffix(path, "/pods"):
		ns := strings.TrimSuffix(strings.TrimPrefix(path, "namespaces/"), "/pods")
		items = podsIn(ns)
	default:
		return nil, false, nil
	}
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return nil, false, fmt.Errorf("encode metrics: %w", err)
	}
	return body, true, nil
}

func podsIn(namespace string) []podMetrics {
	out := make([]podMetrics, 0, 4)
	for _, p := range demoPodMetrics() {
		if p.Metadata.Namespace == namespace {
			out = append(out, p)
		}
	}
	return out
}

func respond(req *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}
}

// newMetricsREST builds a real rest.RESTClient whose transport never leaves
// the process. A negative QPS turns client-side throttling off.
func newMetricsREST(status int) (rest.Interface, error) {
	cfg := &rest.Config{
		Host:    "https://fake.invalid",
		APIPath: "/apis",
		QPS:     -1,
		ContentConfig: rest.ContentConfig{
			NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		},
	}
	rc, err := rest.UnversionedRESTClientForConfigAndClient(cfg, &http.Client{Transport: metricsRoundTripper{status: status}})
	if err != nil {
		return nil, fmt.Errorf("metrics rest client: %w", err)
	}
	return rc, nil
}
