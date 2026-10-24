package logs

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// fallback is a pod that lacks the container the view streams, so it
// streams its first app container instead (containerFor): a service can
// select pods of several workloads, and the picker read only the first.
type fallback struct{ pod, container string }

// noteFallback is fell with pod added when it streams container rather
// than the view's and the view has not said so for it yet.
func (m Model) noteFallback(fell []fallback, pod, container string) []fallback {
	if m.container == "" || container == m.container || slices.Contains(m.join.fellBack, pod) {
		return fell
	}
	return append(fell, fallback{pod: pod, container: container})
}

// toldFallbacks is j once the view has said so for every pod of fell.
func (j joinState) toldFallbacks(fell []fallback) joinState {
	told := slices.Clip(j.fellBack)
	for _, f := range fell {
		told = append(told, f.pod)
	}
	j.fellBack = told
	return j
}

// fallbackText says that the first pod of fell lacks the view's container
// and what it streams instead, and how many more pods did; "" for none.
// The names are made inert.
func (m Model) fallbackText(fell []fallback) string {
	if len(fell) == 0 {
		return ""
	}
	f := fell[0]
	text := "pod " + sanitize.Line(f.pod) + " has no container " + sanitize.Line(m.container) + ": streaming " + sanitize.Line(f.container)
	switch n := len(fell) - 1; {
	case n == 1:
		text += " (and 1 more pod)"
	case n > 1:
		text += " (and " + strconv.Itoa(n) + " more pods)"
	}
	return text
}

// fallbackCmd is the status that says fell, nil for none.
func (m Model) fallbackCmd(fell []fallback) tea.Cmd {
	text := m.fallbackText(fell)
	if text == "" {
		return nil
	}
	return statusCmd(status.Info, text)
}
