package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

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

// fitFacts adds as many facts after head as fit in w columns, joined
// with " · ".
func fitFacts(head string, facts []string, w int) string {
	for n := len(facts); n > 0; n-- {
		if s := head + "  " + strings.Join(facts[:n], " · "); ansi.StringWidth(s) <= w {
			return s
		}
	}
	return head
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
			parts = append(parts, f.attention(markOf(sev)), group)
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

// attention is the verdict word after the worst glyph: "▲ attention".
func (f frame) attention(k mark) string {
	return f.paint(k) + " " + f.verdictWord("attention")
}
