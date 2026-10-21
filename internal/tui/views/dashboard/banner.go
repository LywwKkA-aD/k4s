package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// The banner (panel 0) answers the first two questions: is the cluster OK
// (the verdict row) and what to run next (the next row, a cursor stop per
// suggestion). The verdict row ends with when the snapshot last changed
// and the watch state; the wide layout opens with the k4s wordmark.

// widenNote is all the smallest layout has room to say besides the verdict.
const widenNote = "widen the terminal"

// wordmark is "k4s" in half blocks, two rows; the wide banner only. Its
// first column is the cursor's.
var wordmark = [bannerRows]string{" █ ▄ █ █ ▄▀▀", " █▀▄ ▀▀█ ▄▄▀"}

// wordmarkGap separates the wordmark from the verdict and the next row.
const wordmarkGap = "   "

// bannerLines are the banner's rows for layout l, each exactly l.w wide.
func bannerLines(f frame, l layout) []string {
	if l.kind == kindMinimal {
		if l.banner < 2 {
			return []string{verdictLine(f, l.w, "", f.muted(widenNote)+" ")}
		}
		return []string{verdictLine(f, l.w, "", ""), fitText(" "+f.muted(widenNote), l.w)}
	}
	var marks [bannerRows]string
	if l.kind == kindWide && l.banner >= bannerRows {
		for i, row := range wordmark {
			marks[i] = f.ds.Title.Render(row[len(" "):]) + wordmarkGap
		}
	}
	lines := []string{verdictLine(f, l.w, marks[0], rightNote(f))}
	if l.banner > 1 {
		lines = append(lines, fitText(nextLine(f, marks[1]), l.w))
	}
	return lines
}

// verdictLine is the first banner row: the cursor column, mark (the
// wordmark's row, or nothing), the verdict, and right on the right.
func verdictLine(f frame, w int, mark, right string) string {
	lead := " "
	if f.focus.panel == dash.PanelOverview && f.focus.row == "" {
		lead = styles.CursorMarker
	}
	head := lead + mark
	room := w - ansi.StringWidth(right) - 2 - ansi.StringWidth(head)
	return spread(head+verdict(f, room), right, w)
}

// rightNote ends the verdict row: "◆ changed 14:32:05 · watch 5s" in the
// wide layout, the time alone where the narrow ones keep the watch state
// in the hints, and "‖ paused 14:32" with watch off.
func rightNote(f frame) string {
	var parts []string
	if f.loaded && !f.changed.IsZero() {
		when := f.changed.Format("15:04:05")
		if f.kind == kindWide {
			when = "changed " + when
		}
		parts = append(parts, f.paint(markFact)+" "+when)
	}
	switch {
	case f.paused:
		parts = append(parts, f.muted(strings.TrimSpace("‖ paused "+clock(f.pausedAt))))
	case !f.narrow() && f.hint != "":
		parts = append(parts, f.muted(f.hint))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + " "
}

// clock is t as hh:mm, "" for the zero time.
func clock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}

// verdict is the cluster in a few words: loading, all clear since when
// and what k4s saw, or the chips most severe first with a +N tail for
// those that do not fit in w columns. A partial picture leads with why
// (stale, scoped); without chips it then lists what k4s did see.
func verdict(f frame, w int) string {
	v := f.snap.Verdict
	switch {
	case !f.loaded:
		return f.paint(markFact) + " loading the cluster…"
	case v.AllClear:
		return fitFacts(f.paint(markOK)+" all clear since "+clock(f.since), clearFacts(f.snap), w)
	}
	lead := bannerNotes(f)
	if len(v.Chips) == 0 {
		if len(lead) == 0 {
			return f.paint(markFact) + " nothing failing in what k4s can see"
		}
		return fitChips(strings.Join(lead, "  "), scopeFacts(f), w)
	}
	if f.kind != kindWide {
		return groupedChips(f, lead, v.Chips, w)
	}
	head := markWarn
	if v.Chips[0].Sev == dash.SevFailing {
		head = markErr
	}
	lead = append(lead, f.attention(head))
	chips := make([]string, 0, len(v.Chips))
	for _, c := range v.Chips {
		chips = append(chips, f.paint(markOf(c.Sev))+" "+clean(c.Text))
	}
	return fitChips(strings.Join(lead, "  "), chips, w)
}

// clearFacts are what k4s saw when all is clear: "3/3 nodes Ready · 185
// pods · 65/65 workloads · 2 warnings in 1h", each only where its source
// answered.
func clearFacts(s dash.Snapshot) []string {
	var out []string
	if readable(s.Nodes.Avail) {
		out = append(out, fmt.Sprintf("%d/%d nodes Ready", s.Nodes.Ready, s.Nodes.Total))
	}
	if readable(s.Pods.Avail) {
		out = append(out, plural(s.Pods.Total, "pod", "pods"))
	}
	ready, total, known := 0, 0, false
	for _, k := range []dash.KindSummary{s.Workloads.Deploy, s.Workloads.STS, s.Workloads.DS} {
		if readable(k.Avail) {
			ready, total, known = ready+k.Ready, total+k.Total, true
		}
	}
	if known {
		out = append(out, fmt.Sprintf("%d/%d workloads", ready, total))
	}
	if readable(s.Events.Avail) {
		out = append(out, warningsCount(s.Events)+" in 1h")
	}
	return out
}

// nextLine is the second banner row: the suggestion under the cursor (or
// the first) and the question it answers, or the command of the day when
// nothing needs a fix. The question is never prefixed with #: a pasted
// line must not carry it as arguments. The narrow layouts keep only the
// command. mark is the wordmark's row, or nothing.
func nextLine(f frame, mark string) string {
	text := nextText(f)
	if _, sel := f.selectedSuggestion(); sel {
		return f.ds.Selection.Render(styles.CursorMarker) + mark + f.ds.Selection.Render(ansi.Strip(text))
	}
	return " " + mark + text
}

func nextText(f frame) string {
	if !f.loaded {
		return f.muted("▸ next  waiting for the first answer")
	}
	all := suggestions(f.snap)
	if len(all) == 0 {
		return ""
	}
	i, _ := f.selectedSuggestion()
	i = min(i, len(all)-1)
	label := "▸ command of the day"
	if n := len(f.snap.Next); n > 0 {
		label = nextLabel(f, i, n)
	}
	text := f.ds.Title.Render(label) + "  " + f.command(all[i].Cmd)
	if q := clean(all[i].Question); q != "" && !f.narrow() {
		text += "   " + f.muted(q)
	}
	return text
}
