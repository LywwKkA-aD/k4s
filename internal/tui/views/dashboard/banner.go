package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// The banner (panel 0) answers the first two questions: is the cluster OK
// and what to run next. This is its plain form; the wordmark, the toast
// lane, the heartbeat and cycling through the next suggestions come with
// M7D.10 and M7D.14.

// widenNote is all the smallest layout has room to say besides the verdict.
const widenNote = "widen the terminal"

// bannerLines are the banner's rows for layout l, each exactly l.w wide.
func bannerLines(f frame, l layout) []string {
	if l.kind == kindMinimal {
		if l.banner < 2 {
			return []string{verdictLine(f, l.w, f.muted(widenNote)+" ")}
		}
		return []string{verdictLine(f, l.w, ""), fitText(" "+f.muted(widenNote), l.w)}
	}
	right := f.muted(f.hint) + " "
	if f.narrow() {
		right = "" // the watch state stays in the hints, as "w 5s"
	}
	lines := []string{verdictLine(f, l.w, right)}
	if l.banner > 1 {
		lines = append(lines, fitText(nextLine(f), l.w))
	}
	return lines
}

// verdictLine is the first banner row: the cursor column, the verdict on
// the left and right on the right.
func verdictLine(f frame, w int, right string) string {
	lead := " "
	if f.focus.panel == dash.PanelOverview {
		lead = styles.CursorMarker
	}
	room := w - ansi.StringWidth(right) - 2 - 1
	return spread(lead+verdict(f, room), right, w)
}

// verdict is the cluster in a few words: loading, all clear since when,
// or the chips most severe first with a +N tail for those that do not
// fit in w columns. A partial picture leads with why (stale, scoped);
// without chips it then lists what k4s did see.
func verdict(f frame, w int) string {
	v := f.snap.Verdict
	switch {
	case !f.loaded:
		return f.paint(markFact) + " loading the cluster…"
	case v.AllClear:
		return f.paint(markOK) + " all clear since " + f.since.Format("15:04")
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

// groupedChips is the verdict where the banner is narrower than the wide
// mockup's: chips of one severity share one glyph, "▲ 3 CrashLoopBackOff
// · 1 ImagePullBackOff  △ 2 not ready", and a "· +N" tail counts the
// rest. The medium layout keeps the word "attention" after the worst
// glyph; the narrow ones drop it and shorten the reasons.
func groupedChips(f frame, notes []string, chips []dash.Chip, w int) string {
	for n := len(chips); n > 0; n-- {
		s := chipGroups(f, notes, chips[:n])
		if n < len(chips) {
			s += fmt.Sprintf(" · +%d", len(chips)-n)
		}
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
	return strings.Join(append(slices.Clip(notes), f.paint(markOf(chips[0].Sev))+fmt.Sprintf(" +%d", len(chips))), "  ")
}

// chipGroups lays out chips, most severe first, one glyph per run of
// equal severity.
func chipGroups(f frame, notes []string, chips []dash.Chip) string {
	parts := slices.Clip(notes)
	texts := make([]string, 0, len(chips))
	flush := func(sev dash.Severity, first bool) {
		if len(texts) == 0 {
			return
		}
		group := strings.Join(texts, " · ")
		switch {
		case first && !f.narrow():
			parts = append(parts, f.paint(markOf(sev))+" attention", group)
		default:
			parts = append(parts, f.paint(markOf(sev))+" "+group)
		}
		texts = texts[:0]
	}
	for i, c := range chips {
		if i > 0 && c.Sev != chips[i-1].Sev {
			flush(chips[i-1].Sev, len(parts) == len(notes))
		}
		texts = append(texts, chipText(c, f.narrow()))
	}
	flush(chips[len(chips)-1].Sev, len(parts) == len(notes))
	return strings.Join(parts, "  ")
}

// chipText is a chip's words for a grouped banner: the workloads chips
// lose the word their panel number already says ("4 workloads", "1 job"),
// and the narrow layouts shorten the BackOff reasons.
func chipText(c dash.Chip, narrow bool) string {
	t := clean(c.Text)
	if c.Panel == dash.PanelWorkloads {
		t = strings.TrimSuffix(strings.TrimSuffix(t, " not ready"), " failed")
	}
	if narrow {
		t = strings.NewReplacer("CrashLoopBackOff", "CrashLoop", "ImagePullBackOff", "ImagePull").Replace(t)
	}
	return t
}

// nextLine is the second banner row: the first next suggestion and the
// question it answers, or the command of the day when nothing needs a
// fix. The question is never prefixed with #: a pasted line must not
// carry it as arguments. The narrow layouts keep only the command.
func nextLine(f frame) string {
	s := f.snap
	switch {
	case !f.loaded:
		return " " + f.muted("▸ next  waiting for the first answer")
	case len(s.Next) > 0 && f.narrow():
		return " " + f.ds.Title.Render("▸ next") + "  " + f.command(s.Next[0].Cmd)
	case len(s.Next) > 0:
		n := s.Next[0]
		label := "▸ next"
		if len(s.Next) > 1 {
			label = fmt.Sprintf("▸ next 1/%d", len(s.Next))
		}
		return " " + f.ds.Title.Render(label) + "  " + f.command(n.Cmd) + "   " + f.muted(clean(n.Question))
	case !s.Daily.Cmd.IsZero():
		return " " + f.ds.Title.Render("▸ command of the day") + "  " + f.command(s.Daily.Cmd)
	}
	return ""
}
