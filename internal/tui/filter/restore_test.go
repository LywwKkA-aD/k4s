package filter_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestWithQueryIsACommittedFilter: a restored list shows its filter at
// once, closed, exactly as if the query had been typed and confirmed.
func TestWithQueryIsACommittedFilter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		query      string
		wantQuery  string
		wantActive bool
		wantView   string
	}{
		{name: "query", query: "pay", wantQuery: "pay", wantActive: true, wantView: "/ pay"},
		{name: "surrounding blanks", query: "  pay ", wantQuery: "pay", wantActive: true, wantView: "/ pay"},
		{name: "empty", query: "", wantView: ""},
		{name: "blank", query: "   ", wantView: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := filter.New().WithQuery(tc.query)
			if m.IsOpen() || m.Query() != tc.wantQuery || m.Active() != tc.wantActive || m.View() != tc.wantView {
				t.Fatalf("WithQuery(%q): open=%v query=%q active=%v view=%q", tc.query, m.IsOpen(), m.Query(), m.Active(), m.View())
			}
			if tc.wantActive && (!m.Match("payments") || m.Match("web")) {
				t.Fatalf("WithQuery(%q) does not filter", tc.query)
			}
		})
	}
}

// TestWithQueryThenOpenEditsIt: '/' on a restored filter opens it with the
// query, and Esc clears it, as for a typed one.
func TestWithQueryThenOpenEditsIt(t *testing.T) {
	t.Parallel()

	m, _ := filter.New().WithQuery("pay").Open()
	if !m.IsOpen() || m.Query() != "pay" {
		t.Fatalf("open after WithQuery: open=%v query=%q", m.IsOpen(), m.Query())
	}
	m, _, _ = m.Update(tuitest.Type("x"))
	if m.Query() != "payx" {
		t.Fatalf("typing after WithQuery: query=%q", m.Query())
	}
	m, _, _ = m.Update(tuitest.Esc())
	if m.Active() {
		t.Fatalf("Esc left the filter active: %q", m.Query())
	}
}
