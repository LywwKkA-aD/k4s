package overlay

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

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
// session; the answer is a RestoreDecisionMsg.
func NewRestore(count int, now func() time.Time) Overlay {
	noun := "forwards"
	if count == 1 {
		noun = "forward"
	}
	return NewConfirm(Confirm{
		Title:    "restore port-forwards",
		Question: fmt.Sprintf("Found %d saved %s from a previous session.", count, noun),
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
