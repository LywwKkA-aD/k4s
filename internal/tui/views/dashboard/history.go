package dashboard

import (
	"math"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// History limits. The rings live only in memory, for this session, and
// every chart drawn from them is marked ≈.
const (
	// historyLimit is 60 minutes of node metrics at their 15 s cadence.
	historyLimit = 240
	metricsStep  = 15 * time.Second
	// notOKLimit is 60 minutes of the not-ok count, one point a minute.
	notOKLimit = 60
	notOKStep  = time.Minute
	// maxNodeSeries bounds the per-node rings: past it a node has meters
	// but no strips.
	maxNodeSeries = 20
	// awayAfter is a pause between samples no polling cadence explains
	// (blur, backoff and size stretches stay under it): the dashboard was
	// not on screen, and the missed samples show as blank columns.
	awayAfter = 2 * time.Minute
)

// series is one history ring: the newest samples of one measure, oldest
// first. NaN is a sample k4s does not have. It is a value; push and gap
// return a new series and never write to the receiver's storage.
type series struct {
	vals []float64
}

// values returns the samples, oldest first. The slice is shared: read it,
// never write to it.
func (s series) values() []float64 { return s.vals }

// push returns s with v appended, keeping the newest limit samples.
func (s series) push(v float64, limit int) series {
	keep := s.vals[max(len(s.vals)+1-limit, 0):]
	out := make([]float64, 0, len(keep)+1)
	out = append(out, keep...)
	return series{vals: append(out, v)}
}

// gap returns s with n missing samples appended, at most limit.
func (s series) gap(n, limit int) series {
	n = min(n, limit)
	keep := s.vals[max(len(s.vals)+n-limit, 0):]
	out := make([]float64, 0, len(keep)+n)
	out = append(out, keep...)
	for range n {
		out = append(out, math.NaN())
	}
	return series{vals: out}
}

// nodeHistory is the ≈5m strips of one node.
type nodeHistory struct {
	cpu, mem series
}

// history is what the dashboard draws over time: cluster CPU and memory
// and per-node strips, one point per node metrics sample, and the count
// of pods that are not ok, one point a minute.
type history struct {
	cpu, mem series
	// cpuMin and memMin are the same samples a minute a cell, for the
	// spark rows of the narrower layouts.
	cpuMin, memMin clockMinutes
	nodes          map[string]nodeHistory
	notOK          series

	metricsAt time.Duration // plan time of the last metrics sample taken
	sampledAt time.Time     // its wall time
	firstAt   time.Time     // wall time of the first sample, for "since"
	notOKAt   time.Time
}

// fold takes the samples s brings: a node metrics answer the rings have not
// seen yet, and the not-ok count when a minute has passed.
func (h history) fold(s dash.Snapshot) history {
	h = h.foldMetrics(s)
	return h.foldNotOK(s)
}

func (h history) foldMetrics(s dash.Snapshot) history {
	u := s.Ledger[dash.NodeMetrics]
	if !u.Fetched || u.State != dash.StateOK {
		return h
	}
	if !h.sampledAt.IsZero() && u.At == h.metricsAt {
		return h
	}
	missing := 0
	if !h.sampledAt.IsZero() {
		missing = missed(s.At.Sub(h.sampledAt), metricsStep)
	}
	if h.firstAt.IsZero() {
		h.firstAt = s.At
	}
	h.metricsAt, h.sampledAt = u.At, s.At
	h.cpu = h.cpu.gap(missing, historyLimit).push(percentOf(s.Nodes.CPU), historyLimit)
	h.mem = h.mem.gap(missing, historyLimit).push(percentOf(s.Nodes.Memory), historyLimit)
	h.cpuMin = h.cpuMin.add(s.At, percentOf(s.Nodes.CPU))
	h.memMin = h.memMin.add(s.At, percentOf(s.Nodes.Memory))
	h.nodes = foldNodes(h.nodes, s.Nodes.Rows, missing)
	return h
}

// foldNodes returns the per-node rings with one more sample each, for the
// first maxNodeSeries nodes; a node that left takes its ring with it.
func foldNodes(prev map[string]nodeHistory, rows []dash.NodeRow, missing int) map[string]nodeHistory {
	out := make(map[string]nodeHistory, min(len(rows), maxNodeSeries))
	for _, r := range rows[:min(len(rows), maxNodeSeries)] {
		nh := prev[r.Key.Name]
		out[r.Key.Name] = nodeHistory{
			cpu: nh.cpu.gap(missing, historyLimit).push(percentOf(r.CPU), historyLimit),
			mem: nh.mem.gap(missing, historyLimit).push(percentOf(r.Memory), historyLimit),
		}
	}
	return out
}

func (h history) foldNotOK(s dash.Snapshot) history {
	if s.Ledger[dash.Pods].State != dash.StateOK {
		return h
	}
	if !h.notOKAt.IsZero() && s.At.Sub(h.notOKAt) < notOKStep {
		return h
	}
	missing := 0
	if !h.notOKAt.IsZero() {
		missing = missed(s.At.Sub(h.notOKAt), notOKStep)
	}
	h.notOKAt = s.At
	h.notOK = h.notOK.gap(missing, notOKLimit).push(float64(s.Pods.NotOK), notOKLimit)
	return h
}

// rescoped drops what depends on the namespace listed: the pods series.
// Nodes and cluster meters are cluster-wide and stay.
func (h history) rescoped() history {
	h.notOK, h.notOKAt = series{}, time.Time{}
	return h
}

// missed is how many samples a pause of d skipped at step, counted only
// past awayAfter.
func missed(d, step time.Duration) int {
	if d <= awayAfter {
		return 0
	}
	return int(d/step) - 1
}

// percentOf is a meter as a precise percent, NaN when it is not known.
func percentOf(m dash.Meter) float64 {
	if !m.Known || m.Capacity <= 0 {
		return math.NaN()
	}
	return 100 * float64(m.Used) / float64(m.Capacity)
}
