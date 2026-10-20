package k8stest

import (
	"net/http"
	"slices"
)

const (
	metricsAPI     = "/apis/metrics.k8s.io/v1beta1"
	metricsVersion = "metrics.k8s.io/v1beta1"
	// metrics-server stamps every item with its scrape time and window;
	// fixed values keep responses deterministic.
	sampleTime   = "2026-01-01T00:00:00Z"
	sampleWindow = "15s"
)

// ContainerUsage is one container's usage inside a PodMetrics item.
// Quantities are raw strings ("250m", "64Mi") so tests can feed malformed
// values as well as valid ones.
type ContainerUsage struct {
	Name   string
	CPU    string
	Memory string
}

// PodMetrics is one item of a metrics.k8s.io/v1beta1 PodMetricsList.
type PodMetrics struct {
	Namespace  string
	Name       string
	Containers []ContainerUsage
}

// NodeMetrics is one item of a metrics.k8s.io/v1beta1 NodeMetricsList.
type NodeMetrics struct {
	Name   string
	CPU    string
	Memory string
}

type wireUsage struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type wireMeta struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	CreationTimestamp string `json:"creationTimestamp"`
}

type wireContainer struct {
	Name  string    `json:"name"`
	Usage wireUsage `json:"usage"`
}

type wirePod struct {
	Metadata   wireMeta        `json:"metadata"`
	Timestamp  string          `json:"timestamp"`
	Window     string          `json:"window"`
	Containers []wireContainer `json:"containers"`
}

type wireNode struct {
	Metadata  wireMeta  `json:"metadata"`
	Timestamp string    `json:"timestamp"`
	Window    string    `json:"window"`
	Usage     wireUsage `json:"usage"`
}

type wireList[T any] struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Metadata   struct{} `json:"metadata"`
	Items      []T      `json:"items"`
}

// Metrics serves pods and nodes the way metrics-server does behind the
// aggregator:
//
//	GET /apis/metrics.k8s.io/v1beta1/pods                       every pod
//	GET /apis/metrics.k8s.io/v1beta1/namespaces/{ns}/pods       pods in ns
//	GET /apis/metrics.k8s.io/v1beta1/nodes                      every node
//
// Every other request gets NotFound. The handler keeps its own copy of
// pods and nodes.
func Metrics(pods []PodMetrics, nodes []NodeMetrics) http.Handler {
	podItems := toWirePods(pods)
	nodeList := wireList[wireNode]{Kind: "NodeMetricsList", APIVersion: metricsVersion, Items: toWireNodes(nodes)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+metricsAPI+"/pods", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, podList(podItems, ""))
	})
	mux.HandleFunc("GET "+metricsAPI+"/namespaces/{namespace}/pods", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, podList(podItems, r.PathValue("namespace")))
	})
	mux.HandleFunc("GET "+metricsAPI+"/nodes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, nodeList)
	})
	mux.Handle("/", NotFound())
	return mux
}

// podList filters items to namespace; "" keeps them all.
func podList(items []wirePod, namespace string) wireList[wirePod] {
	kept := slices.DeleteFunc(slices.Clone(items), func(p wirePod) bool {
		return namespace != "" && p.Metadata.Namespace != namespace
	})
	return wireList[wirePod]{Kind: "PodMetricsList", APIVersion: metricsVersion, Items: kept}
}

func toWirePods(pods []PodMetrics) []wirePod {
	out := make([]wirePod, 0, len(pods))
	for _, p := range pods {
		containers := make([]wireContainer, 0, len(p.Containers))
		for _, c := range p.Containers {
			containers = append(containers, wireContainer{Name: c.Name, Usage: wireUsage{CPU: c.CPU, Memory: c.Memory}})
		}
		out = append(out, wirePod{
			Metadata:   wireMeta{Name: p.Name, Namespace: p.Namespace, CreationTimestamp: sampleTime},
			Timestamp:  sampleTime,
			Window:     sampleWindow,
			Containers: containers,
		})
	}
	return out
}

func toWireNodes(nodes []NodeMetrics) []wireNode {
	out := make([]wireNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, wireNode{
			Metadata:  wireMeta{Name: n.Name, CreationTimestamp: sampleTime},
			Timestamp: sampleTime,
			Window:    sampleWindow,
			Usage:     wireUsage{CPU: n.CPU, Memory: n.Memory},
		})
	}
	return out
}
