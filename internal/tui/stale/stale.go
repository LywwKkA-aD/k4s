// Package stale is how the list views answer a failed refresh: the rows of
// the last good list stay on screen under a one-line banner, the error is
// said once in the status row, and r retries. Before any list has
// succeeded there are no rows to keep, and the view shows its error body.
package stale

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// maxReason bounds the error text in the banner, which must stay one line.
const maxReason = 60

// Health is how a polled list is doing: whether any list ever succeeded,
// the error of the last answer, and which error text was already said.
// The zero value is a list that has not answered yet.
type Health struct {
	good bool
	err  error
	said string
}

// Fold takes the answer to a list. A new error text comes back as an
// Error status, once however many ticks repeat it; a success forgets it,
// so the same failure after a recovery is said again.
func (h Health) Fold(err error) (Health, tea.Cmd) {
	h.err = err
	if err == nil {
		h.good, h.said = true, ""
		return h, nil
	}
	text := err.Error()
	if text == h.said {
		return h, nil
	}
	h.said = text
	msg := views.StatusMsg{Level: status.Error, Text: text}
	return h, func() tea.Msg { return msg }
}

// Reset forgets everything: the list the view shows is a different one.
func (h Health) Reset() Health { return Health{} }

// Err is the error of the last answer, nil after a success.
func (h Health) Err() error { return h.err }

// Failed reports an error with no earlier list to keep on screen.
func (h Health) Failed() bool { return h.err != nil && !h.good }

// Stale reports an error over the rows of an earlier list.
func (h Health) Stale() bool { return h.err != nil && h.good }

// Banner is the line above stale rows, empty when they are not stale.
func (h Health) Banner() string {
	if !h.Stale() {
		return ""
	}
	return styles.Warn.Render("stale: " + reason(h.err) + " · r retry")
}

// reason is the short form of err: its first line without the leading
// "list <resource>: " context, cut to maxReason.
func reason(err error) string {
	text, _, _ := strings.Cut(err.Error(), "\n")
	if rest, ok := strings.CutPrefix(text, "list "); ok {
		if _, after, found := strings.Cut(rest, ": "); found {
			text = after
		}
	}
	if r := []rune(text); len(r) > maxReason {
		text = string(r[:maxReason-1]) + "…"
	}
	return text
}
