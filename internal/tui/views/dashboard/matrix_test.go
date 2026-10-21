package dashboard_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// fixtures are the synthetic clusters of the golden matrix (§10.1). The
// busy k3s one has its own goldens, warmed up with ten minutes of
// history, in TestWideGolden and TestLayoutGoldens.
var fixtures = []struct {
	name string
	base func() dash.Snapshot
}{
	{"all_clear", allClear},
	{"ns_scoped_eks", nsScopedEKS},
	{"no_metrics_k3s", noMetricsK3s},
	{"vanilla_kind", vanillaKind},
}

// matrixSizes are the terminal sizes every fixture is pinned at.
var matrixSizes = [][2]int{{207, 58}, {120, 40}, {80, 24}, {60, 14}}

// TestGoldenMatrix pins every fixture at every matrix size: each line as
// wide as the body, as many lines as it is tall. Regenerate with
//
//	go test ./internal/tui/views/dashboard -run Golden -update
func TestGoldenMatrix(t *testing.T) {
	t.Parallel()

	for _, fx := range fixtures {
		for _, size := range matrixSizes {
			name := fmt.Sprintf("%s_%dx%d", fx.name, size[0], size[1])
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				w, h := body(size[0], size[1])
				screen := tuitest.Screen(mount(t, newFeed(fx.base), w, h))
				requireSize(t, screen, w, h)
				tuitest.Golden(t, name, screen+"\n")
			})
		}
	}
}

// sgr matches one SGR escape sequence and captures its parameters.
var sgr = regexp.MustCompile("\x1b\\[([0-9;:]*)m")

// colourSGR reports an SGR sequence in s that sets a colour.
func colourSGR(s string) (string, bool) {
	for _, m := range sgr.FindAllStringSubmatch(s, -1) {
		for _, p := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ':' }) {
			n, err := strconv.Atoi(p)
			if err != nil {
				continue
			}
			if (n >= 30 && n <= 38) || (n >= 40 && n <= 48) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107) {
				return m[0], true
			}
		}
	}
	return "", false
}

// TestMonoMatchesColour is the colour-blind check (§10.2): for every
// fixture and size, the colour render stripped of its escapes equals the
// mono render stripped, so no state rests on colour alone; and NO_COLOR
// renders no colour at all. It swaps the global theme, so it does not
// run in parallel (parallel tests wait until it is done).
func TestMonoMatchesColour(t *testing.T) {
	type shot struct{ name, frame string }
	render := func() []shot {
		out := make([]shot, 0, len(matrixSizes)*(len(fixtures)+1))
		for _, fx := range append(fixtures, struct {
			name string
			base func() dash.Snapshot
		}{"busy_k3s", busyK3s}) {
			for _, size := range matrixSizes {
				w, h := body(size[0], size[1])
				m, _ := press(t, mount(t, newFeed(fx.base), w, h), "3", "down")
				out = append(out, shot{fmt.Sprintf("%s_%dx%d", fx.name, size[0], size[1]), tuitest.Frame(m)})
			}
		}
		return out
	}
	colour := render()
	if _, ok := colourSGR(colour[0].frame); !ok {
		t.Fatal("the colour theme rendered no colour: the comparison would prove nothing")
	}
	if err := styles.Init(func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}); err != nil {
		t.Fatalf("mono theme: %v", err)
	}
	mono := render()
	if err := styles.Init(func(k string) string {
		if k == "K4S_THEME" {
			return "dark"
		}
		return ""
	}); err != nil {
		t.Fatalf("restore the theme: %v", err)
	}
	for i := range colour {
		if tuitest.Strip(colour[i].frame) != tuitest.Strip(mono[i].frame) {
			t.Errorf("%s: colour and mono read differently:\n%s\n--- mono ---\n%s",
				colour[i].name, tuitest.Strip(colour[i].frame), tuitest.Strip(mono[i].frame))
		}
		if seq, ok := colourSGR(mono[i].frame); ok {
			t.Errorf("%s: NO_COLOR rendered a colour: %q", mono[i].name, seq)
		}
	}
}

// requests counts the requests of plans.
func requests(plans []dash.Plan) int {
	n := 0
	for _, p := range plans {
		n += len(p.Due)
	}
	return n
}

// TestRequestBudget is §10.4 as the view drives it: after the first load
// a minute of polling asks for at most 60 requests, a fast-only tick for
// exactly pods and deployments, pod metrics only while panel 7 is on
// screen, and node metrics never once discovery said metrics-server is
// absent.
func TestRequestBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		base       func() dash.Snapshot
		w, h       int
		podMetrics bool
		metrics    bool
	}{
		{name: "wide, panel 7 on screen", base: busyK3s, w: wideW, h: wideH, podMetrics: true, metrics: true},
		{name: "medium, panel 7 hidden", base: busyK3s, w: 120, h: 34, metrics: true},
		{name: "no metrics-server", base: noMetricsK3s, w: wideW, h: wideH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFeed(tt.base)
			m := mount(t, f, tt.w, tt.h)
			first := len(f.calls())
			advance(t, m, f, time.Minute)
			minute := f.calls()[first:]
			if n := requests(minute); n > 60 {
				t.Fatalf("a minute of polling asked for %d requests, want at most 60", n)
			}
			// The first tick catches up on what the first load could not know
			// was there (k3s, panel 7); the one at 10s is fast-only.
			if got := dueOf(minute[1]); got != "pods,deployments" {
				t.Fatalf("a fast-only tick asked for %s", got)
			}
			asked := ""
			for _, p := range minute {
				asked += dueOf(p) + ","
			}
			if got := strings.Contains(asked, dash.PodMetrics.String()); got != tt.podMetrics {
				t.Errorf("pod metrics asked: %v, want %v (%s)", got, tt.podMetrics, asked)
			}
			if got := strings.Contains(asked, dash.NodeMetrics.String()); got != tt.metrics {
				t.Errorf("node metrics asked: %v, want %v", got, tt.metrics)
			}
		})
	}
}

// BenchmarkWideViewCold renders the 207x58 body on the busy fixture with
// ten minutes of history, drawn afresh every time (the cursor moves
// between two rows): the spec's cold budget is 4 ms.
func BenchmarkWideViewCold(b *testing.B) {
	f := newFeed(busyK3s)
	f.vary = true
	m := mount(b, f, wideW, wideH)
	m = advance(b, m, f, 10*time.Minute)
	one, _ := press(b, m, "3", "down")
	two, _ := press(b, one, "down")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i++; i%2 == 0 {
			_ = one.View()
		} else {
			_ = two.View()
		}
	}
}

// BenchmarkWideViewCached is the same body asked for again with nothing
// changed, as after every tick of an idle cluster: the budget is 1 ms.
func BenchmarkWideViewCached(b *testing.B) {
	f := newFeed(busyK3s)
	f.vary = true
	m := mount(b, f, wideW, wideH)
	m = advance(b, m, f, 10*time.Minute)
	m, _ = press(b, m, "3", "down")
	_ = m.View()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
