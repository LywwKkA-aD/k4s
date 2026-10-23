package dash

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Action is what Enter does on a next suggestion.
type Action int

// Actions.
const (
	ActionCopy         Action = iota // no k4s view: y copies the command
	ActionLogsPrevious               // logs view with --previous (M5.5)
	ActionLogs                       // logs view
	ActionDescribe                   // built-in describe
	ActionRunner                     // describe through the M7.1 runner
)

// Suggestion is one "next" line: an exact read-only command and the
// question it answers.
type Suggestion struct {
	Cmd       Cmd
	Question  string
	Action    Action
	Target    ObjectKey
	Container string
	Sev       Severity
	ago       time.Duration // how recent the finding is; unknownAgo sorts last
}

const (
	maxSuggestions = 5
	unknownAgo     = time.Duration(1<<63 - 1)
)

// suggest applies the spec's "next" rules (§4) to a derived snapshot: at
// most five, by severity then recency, one per object. Rows not yet placed
// in a namespace are skipped, since their command would not be exact.
func suggest(s Snapshot) []Suggestion {
	var out []Suggestion
	for _, r := range s.Pods.Problems {
		if sg, ok := podSuggestion(r); ok {
			out = append(out, sg)
		}
	}
	for _, r := range s.Workloads.Rows {
		if sg, ok := workloadSuggestion(r); ok {
			out = append(out, sg)
		}
	}
	for _, r := range s.Nodes.Rows {
		if r.Condition != "Ready" || len(r.Pressures) > 0 {
			out = append(out, Suggestion{Cmd: r.Cmd, Question: "what is wrong with this node?", Action: ActionRunner, Target: r.Key, Sev: r.Sev, ago: unknownAgo})
		}
	}
	for _, e := range s.Events.Rows {
		if sg, ok := probeSuggestion(e); ok {
			out = append(out, sg)
		}
	}
	for _, k := range s.Cluster.Network.PendingObjects {
		out = append(out, Suggestion{
			Cmd: describe("svc", k.Name, k.Namespace), Question: "why is the LoadBalancer pending?",
			Action: ActionDescribe, Target: k, Sev: SevInfo, ago: unknownAgo,
		})
	}
	out = append(out, accessSuggestions(s)...)
	return pick(out)
}

// ownedBy reports whether pod belongs to workload w: its pods are named
// <w>-<hash>-<id> (deployment) or <w>-<ordinal> (statefulset). Matching the
// shape, not just the prefix, keeps api-gateway-7d9f-x from covering "api".
func ownedBy(pod, w ObjectKey) bool {
	if pod.Kind != "pod" || pod.Namespace != w.Namespace {
		return false
	}
	rest, ok := strings.CutPrefix(pod.Name, w.Name+"-")
	if !ok || rest == "" {
		return false
	}
	switch w.Kind {
	case "deployment":
		return strings.Count(rest, "-") == 1 && !strings.HasPrefix(rest, "-") && !strings.HasSuffix(rest, "-")
	case "statefulset":
		return isOrdinal(rest)
	default:
		return false
	}
}

func isOrdinal(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func podSuggestion(r PodRow) (Suggestion, bool) {
	if !r.Resolved {
		return Suggestion{}, false
	}
	ago := r.Age
	if r.HasRestart {
		ago = r.LastRestart
	}
	sg := Suggestion{Target: r.Key, Sev: r.Sev, ago: ago, Action: ActionDescribe, Cmd: r.Cmd}
	switch {
	case r.Class == ClassCrash && r.Restarts > 0 && r.Container != "":
		sg.Cmd = logsCmd(r.Key, r.Container, "--previous")
		sg.Question, sg.Action, sg.Container = "why does it keep crashing?", ActionLogsPrevious, r.Container
	case r.Class == ClassPull:
		sg.Question = "why can't it pull its image?"
	case r.Class == ClassPending && r.Age >= startGrace:
		sg.Question = "why is it not starting?"
	case r.Sev == SevFailing:
		sg.Question = "why is this pod failing?"
	default:
		return Suggestion{}, false
	}
	return sg, true
}

func workloadSuggestion(r WorkloadRow) (Suggestion, bool) {
	if !r.Resolved {
		return Suggestion{}, false
	}
	switch {
	case r.Key.Kind == "deployment" || r.Key.Kind == "statefulset":
		if r.Ready >= r.Desired {
			return Suggestion{}, false
		}
		action := ActionDescribe
		if r.Key.Kind == "statefulset" {
			action = ActionRunner
		}
		return Suggestion{Cmd: r.Cmd, Question: "why is the rollout stuck?", Action: action, Target: r.Key, Sev: r.Sev, ago: unknownAgo}, true
	case r.Key.Kind == "job" && r.Failed:
		return Suggestion{Cmd: r.Cmd, Question: "why did the job fail?", Action: ActionRunner, Target: r.Key, Sev: r.Sev, ago: r.FailedAgo}, true
	default:
		return Suggestion{}, false
	}
}

// probeSuggestion follows a recent probe warning on a pod that has not
// recovered to the container named in the event's fieldPath, so the logs
// command reads the right container.
func probeSuggestion(e EventRow) (Suggestion, bool) {
	live := e.Counted || (e.Flagged && e.Ago < recentEvent)
	if !live || e.Reason != "Unhealthy" || e.Object.Kind != "pod" || e.Container == "" || e.Object.Namespace == "" {
		return Suggestion{}, false
	}
	return Suggestion{
		Cmd:      logsCmd(e.Object, e.Container),
		Question: "why does it fail its probe?", Action: ActionLogs, Target: e.Object, Container: e.Container,
		Sev: SevWaiting, ago: e.Ago,
	}, true
}

// accessSuggestions point at what is missing: a denied list or an absent
// metrics API. They only copy; no k4s view answers them.
func accessSuggestions(s Snapshot) []Suggestion {
	var out []Suggestion
	if deniedList(s.Ledger) {
		out = append(out, Suggestion{Cmd: canIListAll(s.Scope.Namespace), Question: "what am I allowed to see here?",
			Action: ActionCopy, Target: ObjectKey{Kind: "access"}, Sev: SevWaiting})
	}
	if s.Nodes.Metrics.State == StateAbsent {
		out = append(out, Suggestion{Cmd: kubectl("api-resources", "--api-group=metrics.k8s.io"),
			Question: "is the metrics API served?", Action: ActionCopy, Target: ObjectKey{Kind: "metrics"}, Sev: SevInfo})
	}
	return out
}

// canIListAll is `kubectl auth can-i --list [-n <ns>]`, the zero Cmd for
// a namespace kubectl would not take.
func canIListAll(ns string) Cmd {
	switch {
	case ns == "":
		return kubectl("auth", "can-i", "--list")
	case !isLabel(ns):
		return Cmd{}
	}
	return kubectl("auth", "can-i", "--list", "-n", ns)
}

// logsCmd is `kubectl logs <pod> -n <ns> -c <container>` and extra, the
// zero Cmd when a name is not one kubectl takes as written.
func logsCmd(pod ObjectKey, container string, extra ...string) Cmd {
	if !isName(pod.Name) || !isLabel(pod.Namespace) || !isLabel(container) {
		return Cmd{}
	}
	args := append([]string{"logs", pod.Name, "-n", pod.Namespace, "-c", container}, extra...)
	return kubectl(args...)
}

// deniedList reports a denied namespaced list or a denied nodes list; the
// cluster-scoped extras (whoami, readyz) are only hidden rows.
func deniedList(led Ledger) bool {
	for _, e := range Endpoints() {
		if led[e].State == StateForbidden && (e.Namespaced() || e == Nodes) {
			return true
		}
	}
	return false
}

// pick orders by severity, then the most recent, keeps one per object and
// at most five.
func pick(in []Suggestion) []Suggestion {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Sev != in[j].Sev {
			return in[i].Sev > in[j].Sev
		}
		return in[i].ago < in[j].ago
	})
	// A workload is hidden only by a pod suggestion that made the cut, and
	// hiding it can let another pod in; repeat until the set is stable.
	covered := map[ObjectKey]bool{}
	for {
		out := take(in, covered)
		grew := false
		for _, w := range out {
			if !covered[w.Target] && coveredByPod(w.Target, out) {
				covered[w.Target], grew = true, true
			}
		}
		if !grew {
			return out
		}
	}
}

// take keeps the first suggestion per object, up to the cap, skipping
// the workloads in skip and any suggestion whose command a builder
// refused: a question with no command to answer it is not shown.
func take(in []Suggestion, skip map[ObjectKey]bool) []Suggestion {
	seen := map[ObjectKey]bool{}
	var out []Suggestion
	for _, sg := range in {
		if sg.Cmd.IsZero() || seen[sg.Target] || skip[sg.Target] || len(out) >= maxSuggestions {
			continue
		}
		seen[sg.Target] = true
		out = append(out, sg)
	}
	return out
}

func coveredByPod(target ObjectKey, picked []Suggestion) bool {
	if target.Kind != "deployment" && target.Kind != "statefulset" {
		return false
	}
	for _, sg := range picked {
		if ownedBy(sg.Target, target) {
			return true
		}
	}
	return false
}

// String renders a suggestion for golden files: command, then question.
func (s Suggestion) String() string {
	return fmt.Sprintf("%s    # %s", s.Cmd.String(), strings.TrimSpace(s.Question))
}
