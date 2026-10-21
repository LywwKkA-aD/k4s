package dashboard

import (
	"slices"
	"strings"

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
	return render(m.frame(), m.lay)
}

func (m Model) frame() frame {
	ds := styles.Dashboard()
	hint := m.watchHint()
	if m.forced {
		hint = "refreshing…"
	}
	return frame{
		snap:    m.mem.snap,
		loaded:  m.mem.loaded,
		hist:    m.mem.hist,
		focus:   m.mem.focus,
		since:   m.mem.since,
		ctx:     clean(m.client.Context),
		session: m.session,
		hint:    hint,
		th:      themeOf(ds),
		ds:      ds,
	}
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
		body = bodyOf(f, pl.panel, inW, inH)
	}
	box := viz.Box{Title: panelLabel(pl.panel), Info: body.info, Footer: body.footer, Focused: f.focus.panel == pl.panel}
	return box.Render(f.th, pl.w, pl.h, strings.Join(body.lines, "\n"))
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
	}
	return summaryBody(f, p)
}

// drawLine is a panel as one summary line: "   1 cluster    ● api ready
// · …", in the selection style when the panel has focus.
func drawLine(f frame, pl place) string {
	label := padRight(panelLabel(pl.panel), summaryLabelW)
	summary := f.muted("loading…")
	if f.loaded {
		summary = summaryOf(f, pl.panel)
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
		return k3sSummary(f)
	case dash.PanelTopPods:
		return topPodsSummary(f)
	}
	return ""
}
