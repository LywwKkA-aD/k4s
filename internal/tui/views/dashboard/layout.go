package dashboard

import (
	"slices"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// layoutKind is one of the spec's responsive layouts (§3), picked from the
// body size alone.
type layoutKind int

const (
	// kindMinimal is the verdict and "widen the terminal".
	kindMinimal layoutKind = iota
	// kindTiny is the banner and one summary line per panel.
	kindTiny
	// kindCompact is the accordion: one open panel, the rest summary lines.
	kindCompact
	// kindMedium is cluster and nodes, pods, workloads and events, with a
	// k3s strip and top pods hidden.
	kindMedium
	// kindWide is every panel, in rows of 11, 18 and 21.
	kindWide
)

func (k layoutKind) String() string {
	return [...]string{"minimal", "tiny", "compact", "medium", "wide"}[k]
}

// Layout thresholds and row heights from the spec's responsive rules.
const (
	bannerRows   = 2
	wideMinW     = 160
	wideMinH     = 44
	mediumMinW   = 100
	mediumMinH   = 30
	compactMinW  = 60
	compactMinH  = 14
	minimalBelow = 40
	minimalRows  = 6

	wideTop    = 11
	wideMiddle = 18
	wideBottom = 21
	wideK3s    = 9
	mediumRow  = 10
	// minBox is the shortest open panel worth a frame: a border, one line
	// and a border.
	minBox = 3
)

func kindFor(w, h int) layoutKind {
	switch {
	case w < minimalBelow || h < minimalRows:
		return kindMinimal
	case w < compactMinW || h < compactMinH:
		return kindTiny
	case w < mediumMinW || h < mediumMinH:
		return kindCompact
	case w >= wideMinW && h >= wideMinH:
		return kindWide
	default:
		return kindMedium
	}
}

// shown is which optional panels this cluster has: 6 on k3s with
// kube-system visible, 7 with metrics-server.
type shown struct {
	k3s, top bool
}

// place is where one panel goes in the body: a framed box, or a one-line
// summary when line is set.
type place struct {
	panel      dash.Panel
	x, y, w, h int
	line       bool
}

// layout is the geometry of the body for one size: the banner rows on top
// and the panels in reading order. It is rebuilt only when the size or
// the set of panels the cluster has changes.
type layout struct {
	kind   layoutKind
	w, h   int
	banner int
	places []place
	// sh and open are the inputs besides the size it was built from.
	sh   shown
	open dash.Panel
}

// buildLayout lays out a w x h body. expanded is the compact layout's open
// panel.
func buildLayout(w, h int, sh shown, expanded dash.Panel) layout {
	w, h = max(w, 0), max(h, 0)
	l := layout{kind: kindFor(w, h), w: w, h: h, sh: sh, open: expanded}
	switch l.kind {
	case kindWide:
		l.banner, l.places = bannerRows, wide(w, h, sh)
	case kindMedium:
		l.banner, l.places = bannerRows, medium(w, h, sh)
	case kindCompact:
		l.banner, l.places = bannerRows, compact(w, h, sh, expanded)
	case kindTiny:
		l.banner, l.places = tiny(w, h, sh)
	default:
		l.banner = min(h, 2)
	}
	return l
}

func wide(w, h int, sh shown) []place {
	narrow := w / 3
	broad := w - narrow
	rest := h - bannerRows - wideTop
	middle := rest * wideMiddle / (wideMiddle + wideBottom)
	bottom := rest - middle
	y1, y2 := bannerRows, bannerRows+wideTop
	y3 := y2 + middle
	out := []place{
		{panel: dash.PanelCluster, x: 0, y: y1, w: narrow, h: wideTop},
		{panel: dash.PanelNodes, x: narrow, y: y1, w: broad, h: wideTop},
		{panel: dash.PanelPods, x: 0, y: y2, w: broad, h: middle},
		{panel: dash.PanelWorkloads, x: broad, y: y2, w: narrow, h: middle},
	}
	switch {
	case sh.k3s && sh.top:
		k3s := bottom * wideK3s / wideBottom
		return append(out,
			place{panel: dash.PanelEvents, x: 0, y: y3, w: broad, h: bottom},
			place{panel: dash.PanelK3s, x: broad, y: y3, w: narrow, h: k3s},
			place{panel: dash.PanelTopPods, x: broad, y: y3 + k3s, w: narrow, h: bottom - k3s})
	case sh.k3s:
		return append(out,
			place{panel: dash.PanelEvents, x: 0, y: y3, w: broad, h: bottom},
			place{panel: dash.PanelK3s, x: broad, y: y3, w: narrow, h: bottom})
	case sh.top:
		return append(out,
			place{panel: dash.PanelEvents, x: 0, y: y3, w: broad, h: bottom},
			place{panel: dash.PanelTopPods, x: broad, y: y3, w: narrow, h: bottom})
	}
	return append(out, place{panel: dash.PanelEvents, x: 0, y: y3, w: w, h: bottom})
}

// medium is the 120x40 mockup: a k3s strip under the banner, then rows of
// 10, 11 and 10 where the pods row takes any extra height.
func medium(w, h int, sh shown) []place {
	half := w / 2
	y := bannerRows
	var out []place
	if sh.k3s {
		out = append(out, place{panel: dash.PanelK3s, x: 0, y: y, w: w, h: 1, line: true})
		y++
	}
	middle := h - y - 2*mediumRow
	y2, y3 := y+mediumRow, y+mediumRow+middle
	return append(out,
		place{panel: dash.PanelCluster, x: 0, y: y, w: half, h: mediumRow},
		place{panel: dash.PanelNodes, x: half, y: y, w: w - half, h: mediumRow},
		place{panel: dash.PanelPods, x: 0, y: y2, w: w, h: middle},
		place{panel: dash.PanelWorkloads, x: 0, y: y3, w: half, h: mediumRow},
		place{panel: dash.PanelEvents, x: half, y: y3, w: w - half, h: mediumRow},
	)
}

// compact is the accordion: every panel but the open one is a summary
// line. Without room for a box it is the tiny layout under a full banner.
func compact(w, h int, sh shown, expanded dash.Panel) []place {
	panels := present(sh)
	open := expanded
	if !slices.Contains(panels, open) {
		open = dash.PanelPods
	}
	boxH := h - bannerRows - (len(panels) - 1)
	if boxH < minBox {
		return summaryLines(w, h, bannerRows, panels)
	}
	out := make([]place, 0, len(panels))
	y := bannerRows
	for _, p := range panels {
		if p == open {
			out = append(out, place{panel: p, x: 0, y: y, w: w, h: boxH})
			y += boxH
			continue
		}
		out = append(out, place{panel: p, x: 0, y: y, w: w, h: 1, line: true})
		y++
	}
	return out
}

// tiny is the banner and as many summary lines as fit under it; the banner
// keeps its second row only when every panel still gets a line.
func tiny(w, h int, sh shown) (int, []place) {
	panels := present(sh)
	banner := 1
	if h >= bannerRows+len(panels) {
		banner = bannerRows
	}
	return banner, summaryLines(w, h, banner, panels)
}

func summaryLines(w, h, top int, panels []dash.Panel) []place {
	out := make([]place, 0, len(panels))
	for i, p := range panels {
		y := top + i
		if y >= h {
			break
		}
		out = append(out, place{panel: p, x: 0, y: y, w: w, h: 1, line: true})
	}
	return out
}

// present lists the panels this cluster has, in reading order.
func present(sh shown) []dash.Panel {
	out := []dash.Panel{dash.PanelCluster, dash.PanelNodes, dash.PanelPods, dash.PanelWorkloads, dash.PanelEvents}
	if sh.k3s {
		out = append(out, dash.PanelK3s)
	}
	if sh.top {
		out = append(out, dash.PanelTopPods)
	}
	return out
}

// placeOf returns where p is drawn; ok is false when it is not.
func (l layout) placeOf(p dash.Panel) (place, bool) {
	for _, pl := range l.places {
		if pl.panel == p {
			return pl, true
		}
	}
	return place{}, false
}

// visible lists the drawn panels in reading order, which is panel number
// order: the medium layout draws the k3s strip first, yet 6 still comes
// after 5.
func (l layout) visible() []dash.Panel {
	out := make([]dash.Panel, 0, len(l.places))
	for _, pl := range l.places {
		out = append(out, pl.panel)
	}
	slices.Sort(out)
	return out
}

// drawsTopPods reports whether panel 7 is on screen, the condition for
// polling pod metrics.
func (l layout) drawsTopPods() bool {
	_, ok := l.placeOf(dash.PanelTopPods)
	return ok
}

// neighbour is the panel beside from in direction dir (+1 right, -1
// left): a box touching it on that side and sharing rows with it, the
// top-most when several do. Without one, from stays.
func (l layout) neighbour(from dash.Panel, dir int) dash.Panel {
	cur, ok := l.placeOf(from)
	if !ok {
		return from
	}
	best, found := place{}, false
	for _, p := range l.places {
		if !touches(cur, p, dir) || p.y+p.h <= cur.y || p.y >= cur.y+cur.h {
			continue
		}
		if !found || p.y < best.y {
			best, found = p, true
		}
	}
	if !found {
		return from
	}
	return best.panel
}

func touches(cur, p place, dir int) bool {
	if dir > 0 {
		return p.x == cur.x+cur.w
	}
	return p.x+p.w == cur.x
}
