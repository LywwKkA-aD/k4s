package dashboard

import (
	"fmt"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

func TestKindFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		w, h int
		want layoutKind
	}{
		{207, 52, kindWide},
		{160, 44, kindWide},
		{160, 43, kindMedium},
		{159, 52, kindMedium},
		{120, 34, kindMedium},
		{100, 30, kindMedium},
		{99, 30, kindCompact},
		{100, 29, kindCompact},
		{80, 18, kindCompact},
		{60, 14, kindCompact},
		{59, 14, kindTiny},
		{60, 13, kindTiny},
		{40, 6, kindTiny},
		{39, 30, kindMinimal},
		{120, 5, kindMinimal},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			t.Parallel()
			if got := kindFor(tt.w, tt.h); got != tt.want {
				t.Errorf("kindFor(%d, %d) = %v, want %v", tt.w, tt.h, got, tt.want)
			}
		})
	}
}

// TestWideGeometryMatchesTheMockup pins the 207x58 mockup: a 207x52 body,
// rows of 11, 18 and 21 under the 2-row banner, one third and two thirds.
func TestWideGeometryMatchesTheMockup(t *testing.T) {
	t.Parallel()

	l := buildLayout(207, 52, shown{k3s: true, top: true}, dash.PanelPods)
	want := map[dash.Panel]place{
		dash.PanelCluster:   {panel: dash.PanelCluster, x: 0, y: 2, w: 69, h: 11},
		dash.PanelNodes:     {panel: dash.PanelNodes, x: 69, y: 2, w: 138, h: 11},
		dash.PanelPods:      {panel: dash.PanelPods, x: 0, y: 13, w: 138, h: 18},
		dash.PanelWorkloads: {panel: dash.PanelWorkloads, x: 138, y: 13, w: 69, h: 18},
		dash.PanelEvents:    {panel: dash.PanelEvents, x: 0, y: 31, w: 138, h: 21},
		dash.PanelK3s:       {panel: dash.PanelK3s, x: 138, y: 31, w: 69, h: 9},
		dash.PanelTopPods:   {panel: dash.PanelTopPods, x: 138, y: 40, w: 69, h: 12},
	}
	if l.kind != kindWide || l.banner != 2 {
		t.Fatalf("kind %v banner %d, want wide with a 2-row banner", l.kind, l.banner)
	}
	for p, w := range want {
		got, ok := l.placeOf(p)
		if !ok || got != w {
			t.Errorf("panel %d = %+v (%v), want %+v", p, got, ok, w)
		}
	}
}

func TestWideWithoutOptionalPanels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		sh     shown
		events int // width of the events box
		hidden []dash.Panel
	}{
		{name: "k3s only", sh: shown{k3s: true}, events: 138, hidden: []dash.Panel{dash.PanelTopPods}},
		{name: "metrics only", sh: shown{top: true}, events: 138, hidden: []dash.Panel{dash.PanelK3s}},
		{name: "neither", sh: shown{}, events: 207, hidden: []dash.Panel{dash.PanelK3s, dash.PanelTopPods}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := buildLayout(207, 52, tt.sh, dash.PanelPods)
			ev, _ := l.placeOf(dash.PanelEvents)
			if ev.w != tt.events {
				t.Errorf("events width = %d, want %d", ev.w, tt.events)
			}
			for _, p := range tt.hidden {
				if _, ok := l.placeOf(p); ok {
					t.Errorf("panel %d is placed, want hidden", p)
				}
			}
			for _, p := range []dash.Panel{dash.PanelK3s, dash.PanelTopPods} {
				if pl, ok := l.placeOf(p); ok && pl.h != 21 {
					t.Errorf("panel %d alone in its column is %d tall, want 21", p, pl.h)
				}
			}
		})
	}
}

func TestMediumGeometry(t *testing.T) {
	t.Parallel()

	l := buildLayout(120, 34, shown{k3s: true, top: true}, dash.PanelPods)
	want := []place{
		{panel: dash.PanelK3s, x: 0, y: 2, w: 120, h: 1, line: true},
		{panel: dash.PanelCluster, x: 0, y: 3, w: 60, h: 10},
		{panel: dash.PanelNodes, x: 60, y: 3, w: 60, h: 10},
		{panel: dash.PanelPods, x: 0, y: 13, w: 120, h: 11},
		{panel: dash.PanelWorkloads, x: 0, y: 24, w: 60, h: 10},
		{panel: dash.PanelEvents, x: 60, y: 24, w: 60, h: 10},
	}
	if fmt.Sprint(l.places) != fmt.Sprint(want) {
		t.Fatalf("places =\n%v\nwant\n%v", l.places, want)
	}
	if l.drawsTopPods() {
		t.Error("the medium layout hides top pods")
	}
}

func TestCompactOpensTheExpandedPanel(t *testing.T) {
	t.Parallel()

	l := buildLayout(80, 18, shown{k3s: true, top: true}, dash.PanelPods)
	if l.kind != kindCompact {
		t.Fatalf("kind = %v, want compact", l.kind)
	}
	var boxes, lines []dash.Panel
	for _, p := range l.places {
		if p.line {
			lines = append(lines, p.panel)
			continue
		}
		boxes = append(boxes, p.panel)
		if p.h != 18-2-6 {
			t.Errorf("open panel is %d tall, want %d", p.h, 18-2-6)
		}
	}
	if fmt.Sprint(boxes) != "[3]" || fmt.Sprint(lines) != "[1 2 4 5 6 7]" {
		t.Fatalf("boxes %v lines %v, want [3] and the rest as lines", boxes, lines)
	}
}

func TestCompactTooShortFallsBackToSummaryLines(t *testing.T) {
	t.Parallel()

	l := buildLayout(80, 14, shown{k3s: true, top: true}, dash.PanelPods)
	if l.kind != kindCompact {
		t.Fatalf("kind = %v", l.kind)
	}
	l = buildLayout(70, 11, shown{k3s: true, top: true}, dash.PanelPods)
	for _, p := range l.places {
		if !p.line {
			t.Fatalf("a %dx%d body has no room for a box, got %+v", 70, 11, p)
		}
	}
}

// TestPanelsCollapseInOrder: when not every panel fits, they go in the
// spec's order (top pods, k3s, cluster, events, workloads, nodes) and
// pods goes last; the rest stay in panel order.
func TestPanelsCollapseInOrder(t *testing.T) {
	t.Parallel()

	all := present(shown{k3s: true, top: true})
	for n, want := range []string{"[]", "[3]", "[2 3]", "[2 3 4]", "[2 3 4 5]", "[1 2 3 4 5]", "[1 2 3 4 5 6]", "[1 2 3 4 5 6 7]", "[1 2 3 4 5 6 7]"} {
		if got := fmt.Sprint(keep(all, n)); got != want {
			t.Errorf("keep %d = %s, want %s", n, got, want)
		}
	}
}

// TestSummaryLinesCollapseInOrder is the same order in the smallest
// layout: the banner drops to one row first, then panels go.
func TestSummaryLinesCollapseInOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		h    int
		sh   shown
		want string
	}{
		{h: 9, sh: shown{k3s: true, top: true}, want: "[1 2 3 4 5 6 7]"},
		{h: 8, sh: shown{k3s: true, top: true}, want: "[1 2 3 4 5 6 7]"},
		{h: 7, sh: shown{k3s: true, top: true}, want: "[1 2 3 4 5 6]"},
		{h: 6, sh: shown{k3s: true, top: true}, want: "[1 2 3 4 5]"},
		{h: 6, sh: shown{top: true}, want: "[1 2 3 4 5]"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d rows %+v", tt.h, tt.sh), func(t *testing.T) {
			t.Parallel()
			l := buildLayout(50, tt.h, tt.sh, dash.PanelPods)
			if got := fmt.Sprint(l.visible()); got != tt.want {
				t.Fatalf("%d rows show %s, want %s", tt.h, got, tt.want)
			}
			for i, p := range l.places {
				if i > 0 && p.y != l.places[i-1].y+1 {
					t.Fatalf("summary lines leave a gap: %+v", l.places)
				}
			}
		})
	}
}

// TestEveryPlaceFits sweeps the sizes the view must survive: every place
// lies inside the body and no two overlap.
func TestEveryPlaceFits(t *testing.T) {
	t.Parallel()

	for _, sh := range []shown{{}, {k3s: true}, {top: true}, {k3s: true, top: true}} {
		for w := 1; w <= 300; w += 7 {
			for h := 1; h <= 100; h += 3 {
				l := buildLayout(w, h, sh, dash.PanelEvents)
				checkPlaces(t, l)
			}
		}
	}
}

func checkPlaces(t *testing.T, l layout) {
	t.Helper()
	used := make([][]bool, l.h)
	for y := range used {
		used[y] = make([]bool, l.w)
	}
	for y := range min(l.banner, l.h) {
		for x := range l.w {
			used[y][x] = true
		}
	}
	for _, p := range l.places {
		if p.x < 0 || p.y < 0 || p.w <= 0 || p.h <= 0 || p.x+p.w > l.w || p.y+p.h > l.h {
			t.Fatalf("%dx%d: %+v is outside the body", l.w, l.h, p)
		}
		for y := p.y; y < p.y+p.h; y++ {
			for x := p.x; x < p.x+p.w; x++ {
				if used[y][x] {
					t.Fatalf("%dx%d: %+v overlaps another place", l.w, l.h, p)
				}
				used[y][x] = true
			}
		}
	}
}

func TestNeighbour(t *testing.T) {
	t.Parallel()

	l := buildLayout(207, 52, shown{k3s: true, top: true}, dash.PanelPods)
	tests := []struct {
		from dash.Panel
		dir  int
		want dash.Panel
	}{
		{dash.PanelCluster, +1, dash.PanelNodes},
		{dash.PanelNodes, -1, dash.PanelCluster},
		{dash.PanelNodes, +1, dash.PanelNodes},
		{dash.PanelPods, +1, dash.PanelWorkloads},
		{dash.PanelEvents, +1, dash.PanelK3s},
		{dash.PanelK3s, -1, dash.PanelEvents},
		{dash.PanelTopPods, -1, dash.PanelEvents},
		{dash.PanelOverview, +1, dash.PanelOverview},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d by %d", tt.from, tt.dir), func(t *testing.T) {
			t.Parallel()
			if got := l.neighbour(tt.from, tt.dir); got != tt.want {
				t.Errorf("neighbour(%d, %d) = %d, want %d", tt.from, tt.dir, got, tt.want)
			}
		})
	}
}

func TestVisibleOrder(t *testing.T) {
	t.Parallel()

	l := buildLayout(120, 34, shown{k3s: true, top: true}, dash.PanelPods)
	if got := fmt.Sprint(l.visible()); got != "[1 2 3 4 5 6]" {
		t.Fatalf("visible = %s, want reading order without top pods", got)
	}
	if got := fmt.Sprint(buildLayout(30, 4, shown{}, dash.PanelPods).visible()); got != "[]" {
		t.Fatalf("minimal layout shows %s, want no panels", got)
	}
}
