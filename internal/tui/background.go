package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// firstFrameHold is how long the first frame waits for the terminal to say
// what its background is, so a light terminal does not flash the dark
// palette first.
const firstFrameHold = 150 * time.Millisecond

// holdDoneMsg ends the first-frame hold when the terminal has not answered.
type holdDoneMsg struct{}

// WithBackgroundQuery, when ask is set, makes the program ask the terminal
// for its background at start and hold the first frame until the answer
// comes, at most 150 ms. main passes styles.WantsBackground: the theme
// follows the terminal and the terminal can be asked.
func WithBackgroundQuery(ask bool) Option {
	return func(m *Model) { m.holdFrame = ask }
}

// askBackground asks the terminal for its background and arms the end of
// the first-frame hold; nil when the theme does not follow the terminal.
func (m Model) askBackground() tea.Cmd {
	if !m.holdFrame {
		return nil
	}
	return tea.Batch(
		tea.RequestBackgroundColor,
		tea.Tick(firstFrameHold, func(time.Time) tea.Msg { return holdDoneMsg{} }),
	)
}

// onBackground re-themes for the terminal's background (a no-op unless the
// theme follows it) and lets the first frame through.
func (m Model) onBackground(msg tea.BackgroundColorMsg) Model {
	styles.SetBackground(msg.Color)
	m.holdFrame = false
	return m
}
