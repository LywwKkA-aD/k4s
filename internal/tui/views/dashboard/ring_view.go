package dashboard

import (
	"fmt"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	// ringWide is how many changes panel 5 shows in the wide layout; zoomed
	// it shows the whole ring.
	ringWide = 4
	// ringHead is the lines above the ring's rows: a blank and its title.
	ringHead = 2
	// eventsHead is the lines the wide events panel keeps above its table:
	// the last-hour strip and a blank.
	eventsHead = 2
)

// ringShown is how many changes of total panel 5 draws in h lines: some
// in the wide layout, all of them zoomed, none elsewhere, and only while
// the warnings keep their header and minListRows rows.
func ringShown(total int, kind layoutKind, zoom dash.Panel, h int) int {
	limit := 0
	switch {
	case zoom == dash.PanelEvents:
		limit = ringLimit
	case kind == kindWide:
		limit = ringWide
	}
	room := h - eventsHead - (minListRows + 1) - ringHead
	return max(min(total, limit, room), 0)
}

// ringRows are the cursor stops of the changes shown: each describes its
// object.
func ringRows(changes []change) []row {
	seen := map[string]int{}
	out := make([]row, 0, len(changes))
	for _, c := range changes {
		key := distinct(seen, fmt.Sprintf("ring/%d/%s", c.at.Unix(), c.obj.String()))
		out = append(out, row{key: key, cmd: c.cmd(), target: c.obj, placed: c.obj.Namespace != ""})
	}
	return out
}

// ringLines draws changes under their title, the cursor's row selected.
func ringLines(f frame, changes []change) []string {
	if len(changes) == 0 {
		return nil
	}
	objW := 0
	for _, c := range changes {
		objW = max(objW, len([]rune(c.object())))
	}
	title := f.ds.Approx.Render(styles.GlyphApprox) + f.muted(" SEEN BY K4S  state changes between two polls, kept in memory for this session")
	lines := []string{"", "   " + title}
	rows := ringRows(changes)
	sel := f.selectedIn(dash.PanelEvents, rows)
	for i, c := range changes {
		text := f.paint(markOf(c.sev)) + " " + c.at.Format("15:04") + "  " + padRight(c.object(), objW) + "  " +
			clean(c.from) + " → " + clean(c.to)
		lines = append(lines, f.listLine(text, i == sel))
	}
	return lines
}
