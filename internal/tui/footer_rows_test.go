package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// footerBlock returns the footer's top rule and every line under it.
func footerBlock(t *testing.T, m tui.Model) []string {
	t.Helper()
	ls := lines(m)
	for i := len(ls) - 1; i >= 0; i-- {
		if isRule(ls[i]) {
			return ls[i:]
		}
	}
	t.Fatalf("no footer rule:\n%s", tuitest.Screen(m))
	return nil
}

// TestFooterIsTwoRowsAtEveryWidth: the rule, the status-or-hints row, the
// kubectl row and the bottom padding, whatever the width and the view, so a
// long hint or command never pushes the body up.
func TestFooterIsTwoRowsAtEveryWidth(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("deployment shop/payments-worker has 0 replicas ", 8)
	cases := []struct {
		name  string
		input []tea.Msg
	}{
		{name: "dashboard"},
		{name: "pods", input: colon("pods")},
		{name: "describe", input: []tea.Msg{views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}}},
		{name: "logs", input: []tea.Msg{views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Tail: 50}}},
		{name: "command bar", input: tuitest.Keys(":")},
		{name: "long status", input: []tea.Msg{views.StatusMsg{Level: status.Error, Text: long}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.input...)
			for width := 20; width <= 300; width++ {
				m, _ = tuitest.Drive(t, m, tuitest.Resize(width, screenH))
				block := footerBlock(t, m)
				if len(block) != 4 || strings.TrimSpace(block[3]) != "" {
					t.Fatalf("width %d: footer is %d lines, want rule + 2 rows + padding:\n%s", width, len(block), strings.Join(block, "\n"))
				}
				for _, l := range block {
					if w := ansi.StringWidth(l); w > width {
						t.Fatalf("width %d: footer line is %d wide: %q", width, w, l)
					}
				}
				if got := len(lines(m)); got != screenH {
					t.Fatalf("width %d: screen is %d lines, want %d", width, got, screenH)
				}
			}
		})
	}
}

func TestLongFooterRowsEndInAnEllipsis(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), 60, 20)
	m, _ = tuitest.Drive(t, m, views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo})
	block := footerBlock(t, m)
	hints, command := strings.TrimSpace(block[1]), strings.TrimSpace(block[2])
	if !strings.HasPrefix(hints, "q home") || !strings.HasSuffix(hints, "…") {
		t.Errorf("hints row = %q, want it cut with an ellipsis", hints)
	}
	if command != "kubectl get pods -n demo" {
		t.Errorf("command row = %q", command)
	}
}
