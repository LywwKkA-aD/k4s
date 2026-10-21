package dashboard

import (
	"fmt"
	"slices"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const (
	// eventsInfo is panel 5's top border note: what kubectl events lists.
	eventsInfo = "type=Warning · 1h"
	// minMessageW is the narrowest MESSAGE column worth drawing.
	minMessageW = 24
)

// eventCols is one step of the column drop order: MESSAGE, then COUNT,
// then the namespace after the object.
type eventCols struct {
	message, count, ns bool
}

var eventSteps = []eventCols{
	{message: true, count: true, ns: true},
	{count: true, ns: true},
	{ns: true},
	{},
}

// eventsBody draws panel 5: the last hour as a strip, then the newest
// warnings.
func eventsBody(f frame, w, h int) panelBody {
	e := f.snap.Events
	body := panelBody{info: eventsInfo}
	switch {
	case e.Avail.State == dash.StateForbidden:
		body.lines = []string{
			" " + f.paint(markUnknown) + " events forbidden",
			"   " + f.command(e.Avail.CanI),
		}
		return body
	case !readable(e.Avail):
		body.lines = []string{" " + f.unreadable("events", e.Avail)}
		return body
	case len(e.Rows) == 0:
		body.lines = []string{
			" " + f.paint(markOK) + " no warnings in the last hour",
			"   " + f.muted("events live about an hour"),
		}
		return body
	}
	items := eventItems(f.snap)
	var head []string
	if f.kind == kindWide {
		strip := []string{" " + eventsStrip(f, w-2)}
		head = fitHead(h, min(len(items), minListRows)+1, slices.Concat(strip, []string{""}), strip)
	}
	t := fitEvents(items, w-4, eventSteps, true)
	var table []string
	var hidden int
	if f.kind == kindMedium && !slices.Contains(t.head, "MESSAGE") {
		table, hidden = eventPairs(f, w, items, h-len(head))
	} else {
		table, hidden = eventTable(f, w, items, h-len(head), t)
	}
	body.lines = slices.Concat(head, table)
	listed := fmt.Sprintf("newest first · %d of %d", len(items), e.Total)
	if e.Truncated {
		listed = fmt.Sprintf("newest first · %d of ≥%d", len(items), e.Total)
	}
	body.footer = f.listFooter(dash.PanelEvents, len(items), hidden, listed)
	return body
}

// eventsStrip is the ≈ count of warnings over the last hour, one cell per
// two minutes, and the totals.
func eventsStrip(f frame, w int) string {
	e := f.snap.Events
	vals := make([]float64, len(e.LastHour))
	for i, n := range e.LastHour {
		vals[i] = float64(n)
	}
	text := f.ds.Approx.Render(styles.GlyphApprox) + " last hour " + viz.Spark(f.th, len(vals), vals, 0) +
		"   " + warningsCount(e) + " · " + plural(e.Reasons, "reason", "reasons") + " · " + plural(e.Objects, "object", "objects")
	return cutText(text, w)
}

// eventTable is the header and the warnings that fit in room lines, the
// cursor's row kept on screen; hidden counts the rows left out.
func eventTable(f frame, w int, items []eventItem, room int, t eventLayout) (lines []string, hidden int) {
	avail := w - 4
	lines = []string{f.muted("   " + t.c.join(t.head))}
	sel := f.selectedIn(dash.PanelEvents, eventRows(items))
	room = max(room-1, 0)
	start := window(len(items), room, sel)
	for i := start; i < min(len(items), start+room); i++ {
		text := f.paint(eventMark(items[i].e)) + " " + cutText(t.c.join(t.cells[i]), avail)
		lines = append(lines, f.listLine(text, i == sel))
	}
	return lines, len(items) - (len(lines) - 1)
}

// eventPairs is the medium layout's warnings, two lines each: when, why
// and what over the message. They need no header.
func eventPairs(f frame, w int, items []eventItem, room int) (lines []string, hidden int) {
	avail := w - 4
	t := fitEvents(items, avail, eventSteps[1:], false)
	sel := f.selectedIn(dash.PanelEvents, eventRows(items))
	fit := max(room/2, 0)
	start := window(len(items), fit, sel)
	for i := start; i < min(len(items), start+fit); i++ {
		top := f.paint(eventMark(items[i].e)) + " " + cutText(t.c.join(t.cells[i]), avail)
		msg := "    " + cutText(clean(items[i].e.Message), w-6)
		if i == sel {
			lines = append(lines, f.listLine(top, true), f.ds.Selection.Render(" "+msg))
			continue
		}
		lines = append(lines, " "+top, " "+f.muted(msg))
	}
	return lines, len(items) - len(lines)/2
}

func eventRows(items []eventItem) []row {
	out := make([]row, 0, len(items))
	for _, it := range items {
		out = append(out, it.row)
	}
	return out
}

// eventLayout is a measured events table: its columns, header and cells.
type eventLayout struct {
	c     cols
	head  []string
	cells [][]string
}

// fitEvents picks the widest of steps that fits in avail, the last one
// when none does. The message is the last column and is cut at the edge,
// so it fits when it keeps minMessageW columns. Without a header drawn,
// the header sets no width.
func fitEvents(items []eventItem, avail int, steps []eventCols, headed bool) eventLayout {
	var t eventLayout
	for _, step := range steps {
		t.head, t.cells = eventCells(items, step)
		sized := t.head
		if !headed {
			sized = make([]string, len(t.head))
		}
		t.c = measure(sized, t.cells, nil, nil)
		need := t.c.width()
		if step.message {
			need += minMessageW - t.c.widths[len(t.c.widths)-1]
		}
		if need <= avail {
			break
		}
	}
	return t
}

func eventCells(items []eventItem, c eventCols) ([]string, [][]string) {
	head := []string{"LAST SEEN", "REASON", "OBJECT"}
	if c.count {
		head = append(head, "COUNT")
	}
	if c.message {
		head = append(head, "MESSAGE")
	}
	out := make([][]string, 0, len(items))
	for _, it := range items {
		e := it.e
		object := clean(e.Object.Kind + "/" + e.Object.Name)
		if c.ns && e.Object.Namespace != "" {
			object += " · " + clean(e.Object.Namespace)
		}
		cells := []string{ago(e.Ago), clean(e.Reason), object}
		if c.count {
			cells = append(cells, fmt.Sprintf("×%d", e.Count))
		}
		if c.message {
			cells = append(cells, clean(e.Message))
		}
		out = append(out, cells)
	}
	return head, out
}
