package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// eventItem is one row of panel 5 and the warning behind it.
type eventItem struct {
	row
	e dash.EventRow
}

// eventItems are panel 5's rows, newest first as the data layer sorts
// them. A row stands for its object and reason: one object can have
// several warnings.
func eventItems(s dash.Snapshot) []eventItem {
	seen := map[string]int{}
	out := make([]eventItem, 0, len(s.Events.Rows))
	for _, e := range s.Events.Rows {
		key := distinct(seen, e.Object.String()+" "+e.Reason)
		placed := e.Object.Namespace != ""
		out = append(out, eventItem{row: row{key: key, cmd: e.Cmd, target: e.Object, placed: placed}, e: e})
	}
	return out
}

// eventMark is a warning's glyph: △ when it moves the verdict (recent, on
// an object that still exists and that no other panel already flags),
// ◇ otherwise. A warning is never ▲: the failing object carries that.
func eventMark(e dash.EventRow) mark {
	if e.Counted {
		return markWarn
	}
	return markInfo
}

// warningsCount is "10 warnings", "2000+ warnings" for a capped list.
func warningsCount(e dash.EventsPanel) string {
	if e.Truncated {
		return atLeast(e.Total, true) + " warnings"
	}
	return plural(e.Total, "warning", "warnings")
}

// eventsSummary is panel 5 on one line: the warnings of the last hour and
// their most frequent reasons.
func eventsSummary(f frame) string {
	e := f.snap.Events
	switch {
	case !readable(e.Avail):
		return f.unreadable("events", e.Avail)
	case e.Total == 0:
		return f.paint(markOK) + " no warnings in the last hour"
	}
	m := markFact
	if e.Counted > 0 {
		m = markWarn
	}
	head := f.paint(m) + " " + warningsCount(e) + " in 1h"
	return strings.Join(append([]string{head}, topReasons(e.Rows, 3)...), " · ")
}

// topReasons are the first n reasons, newest first, with "×4" where a
// reason has several warnings.
func topReasons(rows []dash.EventRow, n int) []string {
	var order []string
	counts := map[string]int{}
	for _, r := range rows {
		reason := clean(r.Reason)
		if counts[reason] == 0 {
			order = append(order, reason)
		}
		counts[reason]++
	}
	out := make([]string, 0, n)
	for _, reason := range order[:min(n, len(order))] {
		if c := counts[reason]; c > 1 {
			reason += fmt.Sprintf(" ×%d", c)
		}
		out = append(out, reason)
	}
	return out
}
