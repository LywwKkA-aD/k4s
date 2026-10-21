// Package stale is how the list views answer a failed refresh: the rows of
// the last good list stay on screen under a one-line banner, the error is
// said once in the status row, and r retries. Before any list has
// succeeded there are no rows to keep, and the view shows its error body.
//
// Every error is worded by k8s.Classify, so the banner, the status row and
// the error body say the same thing: "forbidden: list pods in demo ·
// kubectl auth can-i list pods -n demo", "API unreachable: 10.0.0.1:6443".
package stale

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
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

// Fold takes the answer to a list. A new error text, as Describe words it,
// comes back as an Error status, once however many ticks repeat it; a
// success forgets it, so the same failure after a recovery is said again.
func (h Health) Fold(err error) (Health, tea.Cmd) {
	h = h.Track(err)
	if err == nil {
		h.said = ""
		return h, nil
	}
	text := Describe(err)
	if text == h.said {
		return h, nil
	}
	h.said = text
	msg := views.StatusMsg{Level: status.Error, Text: text}
	return h, func() tea.Msg { return msg }
}

// Track takes the answer to a list without saying anything, for a view
// that words its own status.
func (h Health) Track(err error) Health {
	h.err = err
	if err == nil {
		h.good = true
	}
	return h
}

// Reset forgets everything: the list the view shows is a different one.
func (h Health) Reset() Health { return Health{} }

// Err is the error of the last answer, nil after a success.
func (h Health) Err() error { return h.err }

// Text is the last error as Describe words it, "" after a success: what an
// error body shows.
func (h Health) Text() string { return Describe(h.err) }

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

// reason is the short form of err: its classified Short without the
// leading "list <resource>: " context, cut to maxReason.
func reason(err error) string {
	text := k8s.Classify(err).Short
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
