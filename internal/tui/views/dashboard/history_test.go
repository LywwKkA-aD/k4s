package dashboard

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

func TestSeriesPushKeepsTheNewest(t *testing.T) {
	t.Parallel()

	var s series
	for i := range 5 {
		s = s.push(float64(i), 3)
	}
	if got := fmt.Sprint(s.values()); got != "[2 3 4]" {
		t.Fatalf("values = %s, want [2 3 4]", got)
	}
}

func TestSeriesPushNeverSharesStorage(t *testing.T) {
	t.Parallel()

	base := series{}.push(1, 10).push(2, 10)
	a := base.push(3, 10)
	b := base.push(4, 10)
	if fmt.Sprint(a.values()) != "[1 2 3]" || fmt.Sprint(b.values()) != "[1 2 4]" {
		t.Fatalf("a = %v, b = %v: pushes on one series must not see each other", a.values(), b.values())
	}
	if fmt.Sprint(base.values()) != "[1 2]" {
		t.Fatalf("base changed to %v", base.values())
	}
}

func TestSeriesGap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		n     int
		limit int
		want  int // NaN samples at the end
		size  int
	}{
		{name: "a short gap", n: 2, limit: 10, want: 2, size: 3},
		{name: "a gap longer than the ring", n: 50, limit: 4, want: 4, size: 4},
		{name: "no gap", n: 0, limit: 10, want: 0, size: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := series{}.push(1, tt.limit).gap(tt.n, tt.limit)
			vals := s.values()
			if len(vals) != tt.size {
				t.Fatalf("len = %d, want %d (%v)", len(vals), tt.size, vals)
			}
			nan := 0
			for _, v := range vals {
				if math.IsNaN(v) {
					nan++
				}
			}
			if nan != tt.want {
				t.Fatalf("NaN samples = %d, want %d (%v)", nan, tt.want, vals)
			}
		})
	}
}

var t0 = time.Date(2026, 10, 21, 14, 2, 0, 0, time.UTC)

// metricsSnap is a snapshot whose node metrics were sampled at plan time
// at, wall time t0+at, with the cluster meters at cpu and mem percent.
func metricsSnap(at time.Duration, cpu, mem int64, notOK int) dash.Snapshot {
	var s dash.Snapshot
	s.At = t0.Add(at)
	s.Ledger[dash.NodeMetrics] = dash.Usage{Fetched: true, At: at, State: dash.StateOK}
	s.Ledger[dash.Pods] = dash.Usage{Fetched: true, At: at, State: dash.StateOK}
	s.Nodes.CPU = dash.Meter{Known: true, Used: cpu, Capacity: 100}
	s.Nodes.Memory = dash.Meter{Known: true, Used: mem, Capacity: 100}
	s.Nodes.Rows = []dash.NodeRow{{
		Key: dash.ObjectKey{Kind: "node", Name: "k3s-cp-1"},
		CPU: dash.Meter{Known: true, Used: cpu, Capacity: 100}, Memory: dash.Meter{Known: true, Used: mem, Capacity: 100},
	}}
	s.Pods.NotOK = notOK
	return s
}

func TestHistoryFoldTakesOnePointPerMetricsSample(t *testing.T) {
	t.Parallel()

	var h history
	h = h.fold(metricsSnap(0, 30, 60, 1))
	h = h.fold(metricsSnap(0, 99, 99, 1)) // the same sample again: no point
	h = h.fold(metricsSnap(15*time.Second, 40, 70, 1))
	if got := fmt.Sprint(h.cpu.values()); got != "[30 40]" {
		t.Fatalf("cpu = %s, want [30 40]", got)
	}
	if got := fmt.Sprint(h.mem.values()); got != "[60 70]" {
		t.Fatalf("mem = %s, want [60 70]", got)
	}
	if got := fmt.Sprint(h.nodes["k3s-cp-1"].cpu.values()); got != "[30 40]" {
		t.Fatalf("node cpu = %s, want [30 40]", got)
	}
	if !h.firstAt.Equal(t0) {
		t.Fatalf("firstAt = %v, want %v", h.firstAt, t0)
	}
}

func TestHistoryFoldMarksTimeAwayAsEmpty(t *testing.T) {
	t.Parallel()

	h := history{}.fold(metricsSnap(0, 30, 60, 0))
	h = h.fold(metricsSnap(5*time.Minute, 40, 70, 0))
	vals := h.cpu.values()
	// 5 minutes at 15 s is 20 steps: 19 missing samples between the two.
	if len(vals) != 21 || vals[0] != 30 || vals[20] != 40 {
		t.Fatalf("cpu = %v, want 30, 19 gaps, 40", vals)
	}
	for _, v := range vals[1:20] {
		if !math.IsNaN(v) {
			t.Fatalf("cpu = %v, want NaN for the time away", vals)
		}
	}
}

func TestHistoryFoldSkipsUnknownSources(t *testing.T) {
	t.Parallel()

	s := metricsSnap(0, 30, 60, 0)
	s.Ledger[dash.NodeMetrics].State = dash.StateAbsent
	s.Ledger[dash.Pods].State = dash.StateForbidden
	h := history{}.fold(s)
	if len(h.cpu.values()) != 0 || len(h.notOK.values()) != 0 {
		t.Fatalf("history took samples from sources that did not answer: cpu %v, not ok %v", h.cpu.values(), h.notOK.values())
	}
}

func TestHistoryNotOKOncePerMinute(t *testing.T) {
	t.Parallel()

	var h history
	for i, n := range []int{3, 4, 5, 6, 7} {
		h = h.fold(metricsSnap(time.Duration(i)*20*time.Second, 10, 10, n))
	}
	// Samples at 0, 20, 40, 60 and 80 s: a point at 0 and at 60 s.
	if got := fmt.Sprint(h.notOK.values()); got != "[3 6]" {
		t.Fatalf("not ok = %s, want [3 6]", got)
	}
}

func TestHistoryKeepsAtMostTwentyNodes(t *testing.T) {
	t.Parallel()

	s := metricsSnap(0, 10, 10, 0)
	s.Nodes.Rows = nil
	for i := range 25 {
		s.Nodes.Rows = append(s.Nodes.Rows, dash.NodeRow{
			Key: dash.ObjectKey{Kind: "node", Name: fmt.Sprintf("n%02d", i)},
			CPU: dash.Meter{Known: true, Used: 1, Capacity: 2},
		})
	}
	h := history{}.fold(s)
	if len(h.nodes) != maxNodeSeries {
		t.Fatalf("node series = %d, want %d", len(h.nodes), maxNodeSeries)
	}
	if _, ok := h.nodes["n20"]; ok {
		t.Fatal("node n20 is past the first twenty and must have no series")
	}
	if v := h.nodes["n00"].mem.values(); len(v) != 1 || !math.IsNaN(v[0]) {
		t.Fatalf("memory without metrics = %v, want one NaN", v)
	}
}

func TestHistoryRescopeDropsOnlyThePodSeries(t *testing.T) {
	t.Parallel()

	h := history{}.fold(metricsSnap(0, 30, 60, 4)).rescoped()
	if len(h.notOK.values()) != 0 || !h.notOKAt.IsZero() {
		t.Fatalf("not ok survived a scope change: %v", h.notOK.values())
	}
	if len(h.cpu.values()) != 1 {
		t.Fatalf("cluster cpu = %v, want the cluster-wide sample kept", h.cpu.values())
	}
}

// TestHistoryMinutesBinOnTheClock: the spark rows bin samples by the
// wall-clock minute they were taken in, so a new sample changes only the
// minute still open, and a closed minute never moves.
func TestHistoryMinutesBinOnTheClock(t *testing.T) {
	t.Parallel()

	h := history{}
	for i, cpu := range []int64{10, 20, 30, 40} { // 14:02:00 to 14:02:45
		h = h.fold(metricsSnap(time.Duration(i)*15*time.Second, cpu, 50, 0))
	}
	if got := fmt.Sprint(h.cpuMin.values()); got != "[25]" {
		t.Fatalf("one open minute: cpu = %s, want [25]", got)
	}
	h = h.fold(metricsSnap(time.Minute+10*time.Second, 80, 50, 0))
	if got := fmt.Sprint(h.cpuMin.values()); got != "[25 80]" {
		t.Fatalf("minute closed: cpu = %s, want [25 80]", got)
	}
	h = h.fold(metricsSnap(time.Minute+25*time.Second, 60, 50, 0))
	if got := fmt.Sprint(h.cpuMin.values()); got != "[25 70]" {
		t.Fatalf("only the open minute moves: cpu = %s, want [25 70]", got)
	}
	if got := fmt.Sprint(h.memMin.values()); got != "[50 50]" {
		t.Fatalf("mem = %s, want [50 50]", got)
	}
}

// TestHistoryMinutesMarkTimeAway: minutes without a sample are blank.
func TestHistoryMinutesMarkTimeAway(t *testing.T) {
	t.Parallel()

	h := history{}.fold(metricsSnap(10*time.Second, 30, 60, 0))
	h = h.fold(metricsSnap(4*time.Minute+10*time.Second, 40, 70, 0))
	vals := h.cpuMin.values()
	if len(vals) != 5 || vals[0] != 30 || vals[4] != 40 {
		t.Fatalf("cpu = %v, want 30, three blank minutes, 40", vals)
	}
	for _, v := range vals[1:4] {
		if !math.IsNaN(v) {
			t.Fatalf("cpu = %v, want NaN for the minutes away", vals)
		}
	}
	far := h.fold(metricsSnap(5*time.Hour, 40, 70, 0))
	if n := len(far.cpuMin.values()); n != minuteLimit {
		t.Fatalf("after hours away %d minutes kept, want the ring's %d", n, minuteLimit)
	}
}
