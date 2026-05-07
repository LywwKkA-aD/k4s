package filter

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMatchSubstringCaseInsensitive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		query  string
		fields []string
		want   bool
	}{
		{name: "empty query matches everything", query: "", fields: []string{"foo"}, want: true},
		{name: "exact match", query: "nginx", fields: []string{"nginx-abc"}, want: true},
		{name: "case-insensitive", query: "NGINX", fields: []string{"nginx-abc"}, want: true},
		{name: "matches second field", query: "demo", fields: []string{"nginx", "k4s-demo"}, want: true},
		{name: "no match", query: "redis", fields: []string{"nginx", "k4s-demo"}, want: false},
		{name: "substring inside field", query: "log", fields: []string{"log-spammer-7c"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := New()
			m.input.SetValue(tc.query)
			m.query = tc.query
			if got := m.Match(tc.fields...); got != tc.want {
				t.Errorf("Match(%q, %v) = %v, want %v", tc.query, tc.fields, got, tc.want)
			}
		})
	}
}

func TestOpenCommitCloseLifecycle(t *testing.T) {
	t.Parallel()

	m := New()
	if m.Active() || m.IsOpen() {
		t.Fatal("fresh filter should not be active or open")
	}

	m, _ = m.Open()
	if !m.IsOpen() {
		t.Fatal("after Open(): IsOpen should be true")
	}

	// Type 'd' + 'e' + 'm' + 'o'.
	for _, r := range "demo" {
		var consumed bool
		m, _, consumed = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if !consumed {
			t.Fatalf("Update(%q) returned consumed=false while open", r)
		}
	}
	if got := m.Query(); got != "demo" {
		t.Fatalf("Query after typing = %q, want %q", got, "demo")
	}

	// Enter commits — query stays, prompt blurred.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.IsOpen() {
		t.Fatal("after Enter: IsOpen should be false")
	}
	if !m.Active() {
		t.Fatal("after Enter on non-empty query: Active should be true")
	}
	if m.Query() != "demo" {
		t.Errorf("after commit Query = %q, want %q", m.Query(), "demo")
	}

	// Re-Open + Esc → close, query wiped.
	m, _ = m.Open()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.IsOpen() || m.Active() || m.Query() != "" {
		t.Fatalf("after Esc: open=%v active=%v query=%q", m.IsOpen(), m.Active(), m.Query())
	}
}

func TestCommitOnEmptyValueClosesFilter(t *testing.T) {
	t.Parallel()

	m := New()
	m, _ = m.Open()
	// Press Enter immediately — no value entered.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Active() || m.IsOpen() {
		t.Fatalf("Enter on empty value should close: active=%v open=%v", m.Active(), m.IsOpen())
	}
}
