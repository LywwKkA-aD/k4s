package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// panelBody is what a panel renderer hands its box: the lines inside and
// the notes in the top and bottom edges.
type panelBody struct {
	lines        []string
	info, footer string
}

// summaryLabelW is the panel label column of a summary line: "4 workloads".
const summaryLabelW = 11

// View renders the body: exactly as wide and as tall as the last
// WindowSizeMsg said, every line padded or cut to the width.
func (m Model) View() string {
	if m.client == nil {
		return disconnectedView(m.loadErr, m.home)
	}
	if m.fetch == nil {
		err := m.mem.srcErr
		if err == nil {
			err = dash.ErrNoClient
		}
		return styles.Warn.Render("dashboard unavailable: " + clean(err.Error()))
	}
	f := m.frame()
	key := keyOf(f, m.lay)
	if body, ok := m.memo.get(key); ok {
		return body
	}
	start := time.Now()
	body := render(f, m.lay)
	m.perf.record(time.Since(start))
	m.memo.put(key, body)
	return body
}

func (m Model) frame() frame {
	ds := styles.Dashboard()
	hint := m.watchHint()
	if m.forced {
		hint = "refreshing…"
	}
	f := frame{
		snap:     m.mem.snap,
		loaded:   m.mem.loaded,
		hist:     m.mem.hist,
		focus:    m.mem.focus,
		since:    m.mem.since,
		ctx:      clean(m.client.Context),
		session:  m.session,
		hint:     hint,
		retry:    m.next,
		paused:   !m.poll.Enabled(),
		pausedAt: m.mem.pausedAt,
		changed:  m.mem.changed,
		prog:     min(max(m.mo.spring.Pos, 0), 1),
		beat:     m.beat(),
		fresh:    m.mem.fresh,
		ring:     m.mem.ring,
		toast:    m.toast,
		glow: glow{
			panel: m.mo.sweepPanel, border: m.mo.sweepLeft,
			verdict: m.mo.verdict, wordmark: m.mo.wordmark,
		},

		topByMemory: m.mem.topByMemory,
		th:          themeOf(ds),
		ds:          ds,
	}
	// The fast tier is the pulse: when pods time out, the cluster is gone.
	if u := m.mem.snap.Ledger[dash.Pods]; u.State == dash.StateStale {
		f.staleSince = m.mem.start.Add(u.StaleSince)
	}
	return f
}

// segment is a run drawn at column x of one body line.
type segment struct {
	x    int
	text string
}

// render composes the banner and every place into l.h lines of l.w
// columns.
func render(f frame, l layout) string {
	if l.w <= 0 || l.h <= 0 {
		return ""
	}
	f.kind, f.zoom = l.kind, l.zoom
	rows := make([][]segment, l.h)
	for y, line := range bannerLines(f, l) {
		rows[y] = []segment{{x: 0, text: line}}
	}
	for _, pl := range l.places {
		lines := []string{drawLine(f, pl)}
		if !pl.line {
			lines = strings.Split(drawBox(f, pl), "\n")
		}
		for i, s := range lines {
			if y := pl.y + i; y < l.h {
				rows[y] = append(rows[y], segment{x: pl.x, text: s})
			}
		}
	}
	out := make([]string, l.h)
	for y, segs := range rows {
		out[y] = joinSegments(segs, l.w)
	}
	return strings.Join(out, "\n")
}

// joinSegments lays segments left to right, filling gaps with spaces, and
// fits the line to w.
func joinSegments(segs []segment, w int) string {
	slices.SortFunc(segs, func(a, b segment) int { return a.x - b.x })
	var b strings.Builder
	col := 0
	for _, s := range segs {
		if s.x > col {
			b.WriteString(strings.Repeat(" ", s.x-col))
			col = s.x
		}
		b.WriteString(s.text)
		col += ansi.StringWidth(s.text)
	}
	return fitText(b.String(), w)
}

// drawBox frames panel pl.
func drawBox(f frame, pl place) string {
	inW, inH := max(pl.w-2, 0), max(pl.h-2, 0)
	body := panelBody{lines: []string{" " + f.muted("loading…")}}
	if f.loaded {
		body = panelOf(f, pl.panel, inW, inH)
	}
	box := viz.Box{Title: panelLabel(pl.panel), Info: body.info, Footer: body.footer, Focused: f.focus.panel == pl.panel}
	out := box.Render(f.th, pl.w, pl.h, strings.Join(body.lines, "\n"))
	if box.Focused && f.glow.border > 0 && f.glow.panel == pl.panel {
		out = sweepEdges(out, box.Title, f.ds, f.glow.border*pl.w/sweepFrames)
	}
	return out
}

// panelOf is panel p's body, dimmed and marked "stale 2m" in its top edge
// while its data is the last answer of a source that stopped answering.
func panelOf(f frame, p dash.Panel, w, h int) panelBody {
	a, ok := staleOf(f.snap, p)
	if !ok {
		return bodyOf(f, p, w, h)
	}
	body := bodyOf(f.dimmed(), p, w, h)
	body.info = joinNotes(body.info, "stale "+ago(a.Stale))
	return body
}

func bodyOf(f frame, p dash.Panel, w, h int) panelBody {
	switch p {
	case dash.PanelCluster:
		return clusterBody(f, w, h)
	case dash.PanelNodes:
		return nodesBody(f, w, h)
	case dash.PanelPods:
		return podsBody(f, w, h)
	case dash.PanelWorkloads:
		return workloadsBody(f, w, h)
	case dash.PanelEvents:
		return eventsBody(f, w, h)
	case dash.PanelK3s:
		return k3sBody(f, h)
	case dash.PanelTopPods:
		return topBody(f, w, h)
	}
	return panelBody{}
}

// drawLine is a panel as one summary line: "   1 cluster    ● api ready
// · …", in the selection style when the panel has focus.
func drawLine(f frame, pl place) string {
	label := padRight(panelLabel(pl.panel), summaryLabelW)
	summary := f.muted("loading…")
	if f.loaded {
		summary = summaryOf(f, pl.panel)
	}
	if a, ok := staleOf(f.snap, pl.panel); ok && f.loaded {
		summary += f.muted(" · stale " + ago(a.Stale))
	}
	if f.focus.panel == pl.panel {
		return fitText(f.ds.Selection.Render(styles.CursorMarker+"  "+label+"  "+ansi.Strip(summary)), pl.w)
	}
	return fitText("   "+f.ds.Title.Render(label)+"  "+summary, pl.w)
}

// summaryOf is panel p on one line.
func summaryOf(f frame, p dash.Panel) string {
	switch p {
	case dash.PanelCluster:
		return clusterSummary(f)
	case dash.PanelNodes:
		return nodesSummary(f)
	case dash.PanelPods:
		return podsSummary(f)
	case dash.PanelWorkloads:
		return workloadsSummary(f)
	case dash.PanelEvents:
		return eventsSummary(f)
	case dash.PanelK3s:
		if f.kind == kindMedium {
			return k3sStrip(f)
		}
		return k3sSummary(f)
	case dash.PanelTopPods:
		return topPodsSummary(f)
	}
	return ""
}

// beat is the banner's heartbeat glyph: the spinner's frame while it
// runs, nothing before the first answer.
func (m Model) beat() string {
	if !m.mem.loaded && !m.mo.spinning {
		return ""
	}
	return m.mo.beatGlyph(m.mem.beat)
}
