package dash

import "slices"

// Needs tags what a command of the day needs from the cluster, so the
// test-cluster run can expect a documented failure where it cannot work
// (kind has no metrics-server and no k3s CRDs).
type Needs int

// Needs.
const (
	NeedsAny Needs = iota
	NeedsMetrics
	NeedsK3s
	NeedsEtcd
)

// Env is what the current cluster offers.
type Env struct {
	Metrics bool
	K3s     bool
	Etcd    bool
}

// Daily is one curated read-only command with the question it answers.
type Daily struct {
	Cmd      Cmd
	Question string
	Needs    Needs
}

func daily(q string, needs Needs, args ...string) Daily {
	return Daily{Cmd: kubectl(args...), Question: q, Needs: needs}
}

// dailyCommands is the curated list behind the command of the day.
var dailyCommands = []Daily{
	daily("which pods are neither running nor done?", NeedsAny, "get", "pods", "-A", "--field-selector=status.phase!=Running,status.phase!=Succeeded"),
	daily("what went wrong lately?", NeedsAny, "events", "-A", "--types=Warning"),
	daily("where does each pod run?", NeedsAny, "get", "pods", "-A", "-o", "wide"),
	daily("what are my nodes running?", NeedsAny, "get", "nodes", "-o", "wide"),
	daily("which images do my deployments run?", NeedsAny, "get", "deploy", "-A", "-o", "wide"),
	daily("which pods started most recently?", NeedsAny, "get", "pods", "-A", "--sort-by=.status.startTime"),
	daily("which resources are cluster-wide?", NeedsAny, "api-resources", "--namespaced=false"),
	daily("what can be listed at all?", NeedsAny, "api-resources", "--verbs=list", "-o", "name"),
	daily("what am I allowed to do here?", NeedsAny, "auth", "can-i", "--list"),
	daily("who does the API think I am?", NeedsAny, "auth", "whoami"),
	daily("is every apiserver check passing?", NeedsAny, "get", "--raw=/readyz?verbose"),
	daily("is the apiserver alive?", NeedsAny, "get", "--raw=/livez?verbose"),
	daily("where is my control plane?", NeedsAny, "cluster-info"),
	daily("which versions do client and server run?", NeedsAny, "version"),
	daily("how are my namespaces labelled?", NeedsAny, "get", "ns", "--show-labels"),
	daily("which pods does each service select?", NeedsAny, "get", "svc", "-A", "-o", "wide"),
	daily("which pod IPs back each service?", NeedsAny, "get", "endpointslices", "-A"),
	daily("which claims are bound?", NeedsAny, "get", "pvc", "-A"),
	daily("which storage class is the default?", NeedsAny, "get", "storageclass"),
	daily("what limits voluntary disruptions?", NeedsAny, "get", "pdb", "-A"),
	daily("which hosts route into the cluster?", NeedsAny, "get", "ingress", "-A"),
	daily("which traffic is restricted?", NeedsAny, "get", "networkpolicy", "-A"),
	daily("what runs on a schedule?", NeedsAny, "get", "cronjobs", "-A"),
	daily("which jobs ran last?", NeedsAny, "get", "jobs", "-A", "--sort-by=.status.startTime"),
	daily("which pods win under pressure?", NeedsAny, "get", "priorityclass"),
	daily("which namespaces have quotas?", NeedsAny, "get", "resourcequota", "-A"),
	daily("which identities do pods run as?", NeedsAny, "get", "serviceaccounts", "-A"),
	daily("which custom resources are installed?", NeedsAny, "get", "crd"),
	daily("are the aggregated APIs available?", NeedsAny, "get", "apiservices"),
	daily("are nodes still heartbeating?", NeedsAny, "get", "leases", "-n", "kube-node-lease"),
	daily("how loaded is each node?", NeedsMetrics, "top", "nodes"),
	daily("which pods use the most memory?", NeedsMetrics, "top", "pods", "-A", "--sort-by=memory"),
	daily("which charts does k3s manage?", NeedsK3s, "get", "helmcharts", "-A"),
	daily("when was etcd last snapshotted?", NeedsEtcd, "get", "etcdsnapshotfiles"),
}

func (d Daily) fits(env Env) bool {
	switch d.Needs {
	case NeedsMetrics:
		return env.Metrics
	case NeedsK3s:
		return env.K3s
	case NeedsEtcd:
		return env.K3s && env.Etcd
	default:
		return true
	}
}

// DailyCommands returns a copy of the curated list, for the test-cluster
// run and the help map.
func DailyCommands() []Daily { return slices.Clone(dailyCommands) }

// CommandOfTheDay picks by day of year among the commands env can run.
func CommandOfTheDay(yearDay int, env Env) Suggestion {
	fit := make([]Daily, 0, len(dailyCommands))
	for _, d := range dailyCommands {
		if d.fits(env) {
			fit = append(fit, d)
		}
	}
	d := fit[(max(yearDay, 1)-1)%len(fit)]
	return Suggestion{Cmd: d.Cmd, Question: d.Question, Action: ActionCopy, Target: ObjectKey{Kind: "daily"}}
}

func envOf(s Snapshot) Env {
	return Env{Metrics: s.Caps.Metrics, K3s: s.Caps.K3s, Etcd: s.Caps.Etcd}
}
