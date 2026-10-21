package logs

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// benchView is a full two-pod buffer, 120x40, searched for query; "e"
// hits every line several times, the worst case of a sync.
func benchView(b *testing.B, wrap bool, query string) Model {
	b.Helper()
	m := New(nil, "ns", []string{"a", "b"}, 100, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m.wrap = wrap
	events := make([]logEvent, 0, views.MaxLogLines)
	for i := range views.MaxLogLines {
		pod := "a"
		if i%2 == 1 {
			pod = "b"
		}
		events = append(events, logEvent{Pod: pod, Line: "GET /api/v1/items?page=7 200 OK in 12.5ms from 10.42.0.17 user-agent=kube-probe/1.36 request-id=8f9e7d6c"})
	}
	next, _ = m.Update(logEventsBatch{tok: m.sess.tok, events: events})
	m = next.(Model)
	m.search = newQuery(query)
	m.computeMatches()
	return m
}

func BenchmarkSync(b *testing.B) {
	for _, query := range []string{"", "e"} {
		for _, wrap := range []bool{false, true} {
			name := "clipped"
			if wrap {
				name = "wrapped"
			}
			b.Run(fmt.Sprintf("%s/query=%q", name, query), func(b *testing.B) {
				m := benchView(b, wrap, query)
				b.ResetTimer()
				for range b.N {
					m.syncViewport()
				}
			})
		}
	}
}

// BenchmarkBatchOfLongLines is one line arriving on a backlog of 64 KiB
// lines, which the byte budget bounds and the layout cache keeps from
// being laid out again.
func BenchmarkBatchOfLongLines(b *testing.B) {
	for _, wrap := range []bool{false, true} {
		b.Run(fmt.Sprintf("wrap=%v", wrap), func(b *testing.B) {
			m := New(nil, "ns", []string{"a"}, 100, "")
			next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = next.(Model)
			m.wrap = wrap
			long := make([]logEvent, 0, 200)
			for _, l := range longLines(0, 200) {
				long = append(long, logEvent{Pod: "a", Line: l})
			}
			next, _ = m.Update(logEventsBatch{tok: m.sess.tok, events: long})
			m = next.(Model)
			one := logEventsBatch{tok: m.sess.tok, events: []logEvent{{Pod: "a", Line: longLines(0, 1)[0]}}}
			b.ResetTimer()
			for range b.N {
				next, _ = m.Update(one)
				m = next.(Model)
			}
		})
	}
}
