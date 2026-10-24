package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Confirm is a yes/no question. Yes is sent on Enter, No on n, q or Esc; a
// nil message just closes. y is not an answer: over a confirm it copies
// the command, as everywhere outside a text prompt.
type Confirm struct {
	Title    string
	Question string
	Hint     string
	Yes      tea.Msg
	No       tea.Msg
}

type confirm struct {
	spec   Confirm
	opened time.Time
	now    func() time.Time
}

// NewConfirm opens spec, reading the time from now. Keys that arrive within
// Grace of opening are dropped.
func NewConfirm(spec Confirm, now func() time.Time) Overlay {
	return confirm{spec: spec, opened: now(), now: now}
}

// NewRestore asks whether to revive count port-forwards saved by an earlier
// session; the answer is a RestoreDecisionMsg. clusters, when given, name
// the clusters they were saved for, one "ctx · host" line each, so the
// user sees which cluster a stock "default" context means.
func NewRestore(count int, now func() time.Time, clusters ...string) Overlay {
	noun := "forwards"
	if count == 1 {
		noun = "forward"
	}
	question := fmt.Sprintf("Found %d saved %s from a previous session.", count, noun)
	if len(clusters) > 0 {
		lines := make([]string, 0, len(clusters))
		for _, c := range clusters {
			lines = append(lines, sanitize.Line(c))
		}
		question = fmt.Sprintf("Found %d saved %s from a previous session, for:\n  %s",
			count, noun, strings.Join(lines, "\n  "))
	}
	return NewConfirm(Confirm{
		Title:    "restore port-forwards",
		Question: question,
		Hint:     "Enter revive all · n / Esc skip · :pf to manage later",
		Yes:      RestoreDecisionMsg{Revive: true},
		No:       RestoreDecisionMsg{Revive: false},
	}, now)
}

// Update answers on Enter, n, q or Esc once the grace period is over.
func (c confirm) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || c.now().Sub(c.opened) < Grace {
		return c, nil
	}
	switch {
	case keys.IsEnter(k):
		return c.answer(c.spec.Yes)
	case keys.IsEsc(k):
		return c.answer(c.spec.No)
	}
	switch k.String() {
	case "n", "N", "q":
		return c.answer(c.spec.No)
	}
	return c, nil
}

func (confirm) answer(msg tea.Msg) (Overlay, tea.Cmd) {
	if msg == nil {
		return nil, nil
	}
	return nil, send(msg)
}

// View is the title, the question and the keys.
func (c confirm) View(width int) string {
	return rows(width,
		styles.PopupTitle.Render(c.spec.Title),
		"",
		c.spec.Question,
		"",
		styles.Hint.Render(c.spec.Hint),
	)
}
