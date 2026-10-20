package tuitest

import (
	"fmt"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Find returns the first message of type T in msgs.
func Find[T any](msgs []tea.Msg) (T, bool) {
	for _, m := range msgs {
		if typed, ok := m.(T); ok {
			return typed, true
		}
	}
	var zero T
	return zero, false
}

// All returns every message of type T in msgs, in order.
func All[T any](msgs []tea.Msg) []T {
	var out []T
	for _, m := range msgs {
		if typed, ok := m.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

// Contains reports whether msgs holds a message deeply equal to want.
func Contains(msgs []tea.Msg, want tea.Msg) bool {
	for _, m := range msgs {
		if reflect.DeepEqual(m, want) {
			return true
		}
	}
	return false
}

// viewsPkg is where the cross-view request messages live.
const viewsPkg = "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Requests returns the cross-view messages in msgs (NamespaceSelectedMsg,
// DescribeRequestMsg, LogsRequestMsg, ...): what a view asks the root to
// do, without the view's private bookkeeping messages.
func Requests(msgs []tea.Msg) []tea.Msg {
	var out []tea.Msg
	for _, m := range msgs {
		if t := reflect.TypeOf(m); t != nil && t.PkgPath() == viewsPkg {
			out = append(out, m)
		}
	}
	return out
}

// Describe lists msgs as "%T %+v" lines for failure messages.
func Describe(msgs []tea.Msg) string {
	if len(msgs) == 0 {
		return "(no messages)"
	}
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		lines = append(lines, fmt.Sprintf("%T %+v", m, m))
	}
	return strings.Join(lines, "\n")
}
