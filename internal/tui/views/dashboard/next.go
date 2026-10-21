package dashboard

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The banner's rows below the verdict: the next suggestions, each an
// exact read-only command and the question it answers, or the command of
// the day when nothing needs a fix (§4, "next" rules). ↑↓ steps through
// them, the second banner row shows the one under the cursor, and Enter
// does what the suggestion says: logs, describe, or nothing but y.

const (
	// nextKeyPrefix keys a suggestion's row by the object it is about, so
	// the cursor stays on that object when a poll reorders them.
	nextKeyPrefix = "next/"
	// dailyKey is the command of the day's row.
	dailyKey = "daily"
	// previousNote is the status that goes with logs asked for with
	// --previous: k4s's logs view streams the current container until
	// M5.5 lands.
	previousNote = "k4s shows the current logs; y copies the --previous command"
)

// suggestions are the banner's suggestions: the next ones, else the
// command of the day, else none.
func suggestions(s dash.Snapshot) []dash.Suggestion {
	switch {
	case len(s.Next) > 0:
		return s.Next
	case !s.Daily.Cmd.IsZero():
		return []dash.Suggestion{s.Daily}
	}
	return nil
}

// nextRows are the banner's cursor stops below the verdict, in order;
// none where the banner has no second row.
func nextRows(s dash.Snapshot, l layout) []row {
	if l.banner < bannerRows || l.kind == kindMinimal {
		return nil
	}
	seen := map[string]int{}
	out := make([]row, 0, len(s.Next))
	for _, sg := range suggestions(s) {
		key := dailyKey
		if len(s.Next) > 0 {
			key = distinct(seen, nextKeyPrefix+sg.Target.String())
		}
		placed := sg.Target.Namespace != ""
		out = append(out, row{key: key, cmd: sg.Cmd, target: sg.Target, placed: placed})
	}
	return out
}

// selectedSuggestion is the suggestion the second banner row shows: the
// one under the cursor, else the first. sel reports the cursor on it.
func (f frame) selectedSuggestion() (i int, sel bool) {
	if f.focus.panel != dash.PanelOverview || f.focus.row == "" {
		return 0, false
	}
	return f.focus.index, true
}

// suggestionAt returns the suggestion under the cursor.
func (m Model) suggestionAt() (dash.Suggestion, bool) {
	f := m.mem.focus
	if f.panel != dash.PanelOverview {
		return dash.Suggestion{}, false
	}
	rows := m.rows(dash.PanelOverview)
	i := slices.IndexFunc(rows, func(r row) bool { return r.key == f.row })
	all := suggestions(m.mem.snap)
	if i < 0 || i >= len(all) {
		return dash.Suggestion{}, false
	}
	return all[i], true
}

// runSuggestion is Enter on a suggestion: logs or describe in k4s, and
// for the rest a status that y copies the command.
func (m Model) runSuggestion(sg dash.Suggestion) tea.Cmd {
	switch sg.Action {
	case dash.ActionLogs, dash.ActionLogsPrevious:
		return logsOfSuggestion(sg)
	case dash.ActionDescribe:
		if r := (row{target: sg.Target, placed: sg.Target.Namespace != ""}); describable(r) {
			return describeCmd(sg.Target)
		}
	}
	return statusCmd(status.Info, noViewText)
}

// logsOfSuggestion opens the tail prompt for the suggestion's pod and
// container, and says that --previous is not in k4s yet.
func logsOfSuggestion(sg dash.Suggestion) tea.Cmd {
	cmd := logsCmd(sg.Target, sg.Container)
	if sg.Action == dash.ActionLogsPrevious {
		return tea.Batch(cmd, statusCmd(status.Info, previousNote))
	}
	return cmd
}

// readsLogs reports a suggestion that Enter and l open logs for.
func readsLogs(sg dash.Suggestion) bool {
	return (sg.Action == dash.ActionLogs || sg.Action == dash.ActionLogsPrevious) &&
		sg.Target.Kind == "pod" && sg.Target.Namespace != ""
}

// suggestionHint is the enter hint on a suggestion, "" where Enter only
// says y copies.
func suggestionHint(sg dash.Suggestion) string {
	switch {
	case readsLogs(sg):
		return "logs"
	case sg.Action == dash.ActionDescribe && describable(row{target: sg.Target, placed: sg.Target.Namespace != ""}):
		return "describe"
	}
	return ""
}

// logsCmd asks the root for the tail prompt of one pod, container "" for
// kubectl's default.
func logsCmd(pod dash.ObjectKey, container string) tea.Cmd {
	req := views.TailPromptRequestMsg{
		Namespace: pod.Namespace,
		Pods:      []string{pod.Name},
		Container: container,
		Target:    views.ResourceRef{Kind: "pod", Namespace: pod.Namespace, Name: pod.Name},
	}
	return func() tea.Msg { return req }
}

// describeCmd asks the root for the describe view of k.
func describeCmd(k dash.ObjectKey) tea.Cmd {
	req := views.DescribeRequestMsg{Kind: k.Kind, Namespace: k.Namespace, Name: k.Name}
	return func() tea.Msg { return req }
}

// noViewText is the status for a row whose command has no k4s view.
const noViewText = "no k4s view · y copies the command"

// nextLabel is "▸ next 2/5", or "▸ next" alone in the narrow layouts or
// with one suggestion.
func nextLabel(f frame, i, n int) string {
	if n < 2 || f.narrow() {
		return "▸ next"
	}
	return fmt.Sprintf("▸ next %d/%d", i+1, n)
}
