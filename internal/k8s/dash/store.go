package dash

import (
	"maps"
	"time"
)

// store is the last good payload of every endpoint, decoded to the few
// fields the panels read. Slices in it are never written after decoding,
// so copies of a store share them safely.
type store struct {
	pods         []podRow
	podsMissing  []string
	deploys      []workloadRow
	sts          []workloadRow
	ds           []workloadRow
	cronjobs     []cronRow
	pvcs         []string // STATUS of each claim
	namespaces   []nsRow
	services     []svcRow // the services Table, on servers without the spec.type selector
	lbs          []svcRow // LoadBalancer services
	svcCount     int
	svcCountOK   bool
	svcCountMore bool // the count's list was cut with no remaining count: svcCount is a lower bound
	helm         []helmRow
	snapshots    []snapRow
	nodes        []nodeInfo
	events       []eventInfo
	eventsTotal  int
	eventsCapped bool
	jobs         []jobInfo
	readyz       readyzInfo
	version      versionInfo
	versionGiven bool // the caller hands the version over (GiveVersion)
	user         string
	metricsOK    bool // metrics.k8s.io/v1beta1 is in discovery
	nodeMetrics  []usageRow
	podMetrics   []usageRow
	lbFallback   bool // sticky: this server rejects spec.type field selectors
	resolved     map[resolveKey]resolveEntry
}

// clearers drop an endpoint's payload after a denial or a 404, so a panel
// never shows old numbers as if they were current.
var clearers = map[Endpoint]func(*store){
	Pods:             func(s *store) { s.pods, s.podsMissing = nil, nil },
	Deployments:      func(s *store) { s.deploys = nil },
	StatefulSets:     func(s *store) { s.sts = nil },
	DaemonSets:       func(s *store) { s.ds = nil },
	CronJobs:         func(s *store) { s.cronjobs = nil },
	PVCs:             func(s *store) { s.pvcs = nil },
	Namespaces:       func(s *store) { s.namespaces = nil },
	HelmCharts:       func(s *store) { s.helm = nil },
	EtcdSnapshots:    func(s *store) { s.snapshots = nil },
	LoadBalancers:    func(s *store) { s.lbs, s.services = nil, nil },
	ServiceCount:     func(s *store) { s.svcCount, s.svcCountOK, s.svcCountMore = 0, false, false },
	Nodes:            func(s *store) { s.nodes = nil },
	Events:           func(s *store) { s.events, s.eventsTotal, s.eventsCapped = nil, 0, false },
	Jobs:             func(s *store) { s.jobs = nil },
	NodeMetrics:      func(s *store) { s.nodeMetrics = nil },
	PodMetrics:       func(s *store) { s.podMetrics = nil },
	Readyz:           func(s *store) { s.readyz = readyzInfo{} },
	Version:          func(s *store) { s.version = versionInfo{} },
	Whoami:           func(s *store) { s.user = "" },
	MetricsDiscovery: func(s *store) { s.metricsOK = false },
}

func clearData(s *store, e Endpoint) {
	if drop, ok := clearers[e]; ok {
		drop(s)
	}
}

// clone copies the map the fetcher writes into; slices are shared.
func (s store) clone() store {
	out := s
	out.resolved = maps.Clone(s.resolved)
	if out.resolved == nil {
		out.resolved = map[resolveKey]resolveEntry{}
	}
	return out
}

type podRow struct {
	name        string
	namespace   string // set only when the list was namespaced
	ready       int
	readyOf     int
	readyOK     bool
	status      string
	restarts    int
	lastRestart time.Time
	created     time.Time
	node        string
}

type workloadRow struct {
	name      string
	namespace string
	ready     int
	desired   int
	upToDate  int
	available int
	readyOK   bool
}

type cronRow struct {
	name         string
	namespace    string
	suspend      bool
	lastSchedule time.Time
}

type nsRow struct {
	name    string
	status  string
	created time.Time
}

type svcRow struct {
	name      string
	namespace string
	typ       string
	pending   bool
}

type helmRow struct {
	name string
	job  string
}

type snapRow struct {
	node    string
	created time.Time
}

type nodeInfo struct {
	name          string
	roles         []string
	ready         string // True, False or Unknown
	pressures     []string
	unschedulable bool
	cpuMilli      int64
	memBytes      int64
	podsCap       int64
	created       time.Time
	kubelet       string
	etcd          bool
	controlPlane  bool
	reconciledAt  string // etcd.k3s.cattle.io/local-snapshots-timestamp, the only annotation kept
}

type eventInfo struct {
	kind, namespace, name string
	fieldPath             string
	reason, message       string
	count                 int32
	last                  time.Time
}

type jobInfo struct {
	name, namespace string
	active          int32
	failed          bool
	failedAt        time.Time
}

type readyzInfo struct {
	known  bool
	ready  bool
	checks int
	passed int
	failed []string
	etcd   State
}

type versionInfo struct {
	git   string
	minor int
}

type usageRow struct {
	name, namespace string
	cpuMilli        int64
	memBytes        int64
}
