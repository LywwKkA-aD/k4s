package dash

// Endpoint is one request family the dashboard polls. Each one has a tier,
// a ledger entry and, when it is namespaced, follows the dashboard scope.
type Endpoint int

// The endpoints, in the order of the spec's request table (§8).
const (
	Pods             Endpoint = iota // pods Table, includeObject=None
	Deployments                      // deployments Table
	NodeMetrics                      // metrics.k8s.io nodes
	StatefulSets                     // statefulsets Table
	DaemonSets                       // daemonsets Table
	PodMetrics                       // metrics.k8s.io pods, only while panel 7 shows
	Nodes                            // nodes, typed
	Events                           // events type=Warning, typed
	PVCs                             // persistentvolumeclaims Table
	Jobs                             // jobs status.successful=0, typed
	CronJobs                         // cronjobs Table
	LoadBalancers                    // services spec.type=LoadBalancer, typed; services Table on old servers
	ServiceCount                     // services limit=1, RV=""
	Readyz                           // /readyz?verbose
	Namespaces                       // namespaces Table
	HelmCharts                       // helm.cattle.io helmcharts Table, k3s only
	EtcdSnapshots                    // k3s.cattle.io etcdsnapshotfiles Table, k3s with etcd only
	Version                          // /version
	Whoami                           // SelfSubjectReview
	MetricsDiscovery                 // metrics.k8s.io/v1beta1 in discovery

	numEndpoints
)

// Tier is how often an endpoint is due before adaptive stretching.
type Tier int

// Tiers.
const (
	Fast   Tier = iota // every tick, 5 s
	Medium             // 15 s
	Slow               // 30 s
	Once               // once per context, and on r
)

var endpointNames = [numEndpoints]string{
	"pods", "deployments", "node metrics", "statefulsets", "daemonsets", "pod metrics", "nodes",
	"events", "persistentvolumeclaims", "jobs", "cronjobs", "loadbalancers", "service count",
	"readyz", "namespaces", "helmcharts", "etcdsnapshotfiles", "version", "selfsubjectreview",
	"metrics discovery",
}

// String names the endpoint for logs and tests.
func (e Endpoint) String() string {
	if e < 0 || e >= numEndpoints {
		return "unknown"
	}
	return endpointNames[e]
}

// Tier returns the endpoint's base tier.
func (e Endpoint) Tier() Tier {
	switch e {
	case Pods, Deployments:
		return Fast
	case NodeMetrics, StatefulSets, DaemonSets, PodMetrics:
		return Medium
	case Version, Whoami, MetricsDiscovery:
		return Once
	default:
		return Slow
	}
}

// Namespaced reports whether the endpoint follows the dashboard scope: with
// a namespace it is requested under /namespaces/<ns>/.
func (e Endpoint) Namespaced() bool {
	switch e {
	case Pods, Deployments, StatefulSets, DaemonSets, PodMetrics, Events, PVCs, Jobs,
		CronJobs, LoadBalancers, ServiceCount, HelmCharts:
		return true
	default:
		return false
	}
}

// Endpoints lists every endpoint in request-table order.
func Endpoints() []Endpoint {
	out := make([]Endpoint, 0, numEndpoints)
	for e := range numEndpoints {
		out = append(out, e)
	}
	return out
}
