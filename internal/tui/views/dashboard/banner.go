package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// The banner (panel 0) answers the first two questions: is the cluster OK
// and what to run next. This is its plain form; the wordmark, the toast
// lane, the heartbeat and cycling through the next suggestions come with
// M7D.10 and M7D.14.

// bannerLines are the banner's rows for layout l, each exactly l.w wide.
func bannerLines(f frame, l layout) []string {
	if l.kind == kindMinimal {
		lines := []string{fitText(verdictLine(f, l.w), l.w), fitText(" "+f.muted("widen the terminal"), l.w)}
		return lines[:l.banner]
	}
	lines := []string{verdictLine(f, l.w)}
	if l.banner > 1 {
		lines = append(lines, fitText(nextLine(f), l.w))
	}
	return lines
}

// verdictLine is the first banner row: the cursor column, the verdict on
// the left and the watch state on the right.
func verdictLine(f frame, w int) string {
	right := f.muted(f.hint) + " "
	lead := " "
	if f.focus.panel == dash.PanelOverview {
		lead = styles.CursorMarker
	}
	room := w - ansi.StringWidth(right) - 2 - 1
	return spread(lead+verdict(f, room), right, w)
}

// verdict is the cluster in a few words: loading, all clear since when,
// or the chips most severe first with a +N tail for those that do not
// fit in w columns.
func verdict(f frame, w int) string {
	v := f.snap.Verdict
	switch {
	case !f.loaded:
		return f.paint(markFact) + " loading the cluster…"
	case v.AllClear:
		return f.paint(markOK) + " all clear since " + f.since.Format("15:04")
	}
	var lead []string
	if f.snap.Scope.Fallback {
		lead = append(lead, f.paint(markFact)+" ns "+clean(f.snap.Scope.Namespace)+" (cluster-wide list forbidden)")
	}
	if len(v.Chips) == 0 {
		if len(lead) == 0 {
			lead = append(lead, f.paint(markFact)+" nothing failing in what k4s can see")
		}
		return strings.Join(lead, "  ")
	}
	head := markWarn
	if v.Chips[0].Sev == dash.SevFailing {
		head = markErr
	}
	lead = append(lead, f.paint(head)+" attention")
	chips := make([]string, 0, len(v.Chips))
	for _, c := range v.Chips {
		chips = append(chips, f.paint(markOf(c.Sev))+" "+clean(c.Text))
	}
	return fitChips(strings.Join(lead, "  "), chips, w)
}

// fitChips adds as many chips after lead as fit in w columns, then "+N"
// for the rest.
func fitChips(lead string, chips []string, w int) string {
	for n := len(chips); n >= 0; n-- {
		s := strings.Join(append([]string{lead}, chips[:n]...), "  ")
		if n < len(chips) {
			s += fmt.Sprintf("  +%d", len(chips)-n)
		}
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
	return lead
}

// nextLine is the second banner row: the first next suggestion and the
// question it answers, or the command of the day when nothing needs a
// fix. The question is never prefixed with #: a pasted line must not
// carry it as arguments.
func nextLine(f frame) string {
	s := f.snap
	switch {
	case !f.loaded:
		return " " + f.muted("▸ next  waiting for the first answer")
	case len(s.Next) > 0:
		n := s.Next[0]
		label := "▸ next"
		if len(s.Next) > 1 {
			label = fmt.Sprintf("▸ next 1/%d", len(s.Next))
		}
		return " " + f.ds.Title.Render(label) + "  " + toKubectl(n.Cmd).String() + "   " + f.muted(clean(n.Question))
	case !s.Daily.Cmd.IsZero():
		return " " + f.ds.Title.Render("▸ command of the day") + "  " + toKubectl(s.Daily.Cmd).String()
	}
	return ""
}
