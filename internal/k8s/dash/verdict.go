package dash

import (
	"fmt"
	"sort"
	"strings"
)

// Panel numbers never change with the terminal size; 0 is the banner.
type Panel int

// Panels.
const (
	PanelOverview Panel = iota
	PanelCluster
	PanelNodes
	PanelPods
	PanelWorkloads
	PanelEvents
	PanelK3s
	PanelTopPods
)

// Chip is one verdict item: a count and a short label, without the glyph,
// which the view takes from Sev.
type Chip struct {
	Sev   Severity
	Panel Panel // where Enter on the verdict row goes when this chip leads
	Count int
	Text  string
}

// Verdict is the banner's first line. AllClear needs the whole picture (no
// scope fallback, pods, nodes and deployments readable) and no ▲ or △
// chip; "since" is the view's to remember.
type Verdict struct {
	AllClear bool
	Partial  bool   // part of the cluster is hidden: the banner shows the scope note instead
	Chips    []Chip // most severe first
	Focus    Panel  // the panel of the leading chip
	Cmd      Cmd    // ≈ kubectl get nodes,pods,deploy,sts,ds -A
}

func verdictOf(s Snapshot) Verdict {
	v := Verdict{Cmd: approx(withScope(s.Scope.Namespace, "get", "nodes,pods,deploy,sts,ds"))}
	if s.Cluster.API.Sev == SevFailing {
		v.Chips = append(v.Chips, Chip{Sev: SevFailing, Panel: PanelCluster, Count: len(s.Cluster.API.Failed), Text: "api not ready"})
	}
	v.Chips = append(v.Chips, nodeChips(s.Nodes)...)
	v.Chips = append(v.Chips, podChips(s.Pods)...)
	v.Chips = append(v.Chips, workloadChips(s.Workloads)...)
	if st := s.Cluster.Storage; st.Sev == SevWaiting {
		v.Chips = append(v.Chips, Chip{Sev: SevWaiting, Panel: PanelCluster, Count: st.Pending + st.Lost, Text: plural(st.Pending+st.Lost, "pvc", "pvcs") + " not bound"})
	}
	if n := s.Events.Counted; n > 0 {
		v.Chips = append(v.Chips, Chip{Sev: SevWaiting, Panel: PanelEvents, Count: n, Text: warningsChip(n, s.Events.Truncated)})
	}
	sort.SliceStable(v.Chips, func(i, j int) bool { return v.Chips[i].Sev > v.Chips[j].Sev })
	v.Partial = s.Scope.Fallback || s.Events.Truncated || !sourcesFresh(s)
	v.AllClear = len(v.Chips) == 0 && s.Pods.Avail.State == StateOK && !v.Partial
	if len(v.Chips) > 0 {
		v.Focus = v.Chips[0].Panel
	}
	return v
}

// sourcesFresh reports whether every list the verdict leans on answered:
// a stale, failed, absent or denied source says nothing about its part of
// the cluster, so the banner must not claim all clear over it.
func sourcesFresh(s Snapshot) bool {
	for _, e := range []Endpoint{Nodes, Deployments, Events, Readyz} {
		if s.Ledger[e].State != StateOK {
			return false
		}
	}
	return true
}

func nodeChips(p NodesPanel) []Chip {
	var out []Chip
	if down := p.Total - p.Ready; p.Total > 0 && down > 0 {
		out = append(out, Chip{Sev: SevFailing, Panel: PanelNodes, Count: down, Text: plural(down, "node", "nodes") + " not ready"})
	}
	for _, r := range p.Rows {
		if r.Condition != "Ready" {
			continue
		}
		for _, pr := range r.Pressures {
			out = append(out, Chip{Sev: SevWaiting, Panel: PanelNodes, Count: 1, Text: r.Key.Name + " " + pr})
		}
		for _, m := range []struct {
			name string
			m    Meter
		}{{"cpu", r.CPU}, {"mem", r.Memory}} {
			if m.m.Sev == SevWaiting {
				out = append(out, Chip{Sev: SevWaiting, Panel: PanelNodes, Count: 1, Text: fmt.Sprintf("%s %s %d%%", r.Key.Name, m.name, m.m.Percent)})
			}
		}
	}
	return out
}

func podChips(p PodsPanel) []Chip {
	out := make([]Chip, 0, len(p.Failing)+1)
	for _, rc := range p.Failing {
		out = append(out, Chip{Sev: SevFailing, Panel: PanelPods, Count: rc.Count, Text: fmt.Sprintf("%d %s", rc.Count, rc.Reason)})
	}
	if waiting := p.Waiting(); waiting > 0 {
		out = append(out, Chip{Sev: SevWaiting, Panel: PanelPods, Count: waiting, Text: fmt.Sprintf("%d not ready", waiting)})
	}
	return out
}

func workloadChips(p WorkloadsPanel) []Chip {
	var out []Chip
	notReady, worst := 0, SevOK
	for _, r := range p.Rows {
		if r.Key.Kind == "job" || r.Key.Kind == "cronjob" {
			continue
		}
		notReady++
		worst = max(worst, r.Sev)
	}
	if notReady > 0 {
		out = append(out, Chip{Sev: worst, Panel: PanelWorkloads, Count: notReady, Text: plural(notReady, "workload", "workloads") + " not ready"})
	}
	if n := p.Jobs.FailedRecent; n > 0 {
		out = append(out, Chip{Sev: SevFailing, Panel: PanelWorkloads, Count: n, Text: plural(n, "job", "jobs") + " failed"})
	}
	return out
}

// plural renders "1 job" or "3 jobs".
// warningsChip is the warnings chip: "3 warnings in 10m", "3+ warnings in
// 10m" when they were counted on a page cut at the cap, a lower bound.
func warningsChip(n int, capped bool) string {
	if capped {
		return fmt.Sprintf("%d+ warnings in 10m", n)
	}
	return plural(n, "warning", "warnings") + " in 10m"
}

func plural(n int, one, many string) string {
	word := many
	if n == 1 {
		word = one
	}
	return fmt.Sprintf("%d %s", n, word)
}

// String renders the chips as text, for logs and golden files.
func (v Verdict) String() string {
	if v.AllClear {
		return "all clear"
	}
	parts := make([]string, 0, len(v.Chips)+1)
	if v.Partial {
		parts = append(parts, "partial")
	}
	for _, c := range v.Chips {
		parts = append(parts, fmt.Sprintf("[%s] %s", sevWord(c.Sev), c.Text))
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, "  ")
}

func sevWord(s Severity) string {
	return [...]string{"ok", "info", "minor", "waiting", "failing"}[s]
}
