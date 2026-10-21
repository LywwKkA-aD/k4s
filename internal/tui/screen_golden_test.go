package tui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onTerminal is screen as a terminal width columns wide shows it: lines cut
// at the edge, trailing blanks dropped.
func onTerminal(screen string, width int) string {
	lines := strings.Split(screen, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "")
	}
	return tuitest.TrimLines(strings.Join(lines, "\n"))
}

// TestNarrowScreenGoldens pins whole ANSI-stripped screens on terminals
// narrower than the tables, where rows, the selected row and the footer
// wrap. Regenerate with
//
//	go test ./internal/tui -run TestNarrowScreenGoldens -update
func TestNarrowScreenGoldens(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name  string
		drive func(t *testing.T, m tui.Model) tui.Model
	}{
		{name: "pods", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, colon("pods")...)
			return m
		}},
		{name: "pods_cursor", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, then(colon("pods"), tuitest.Keys("down", "down"))...)
			return m
		}},
		{name: "pods_filter", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, then(colon("pods"), tuitest.FilterBy("web"))...)
			return m
		}},
		{name: "help", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, tuitest.Keys("?")...)
			return m
		}},
		{name: "command_bar", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, tuitest.Press(":"))
			return m
		}},
		{name: "tail_prompt_empty", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, views.TailPromptRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}})
			m, _ = tuitest.Drive(t, m, tuitest.Press("ctrl+u"))
			return m
		}},
		{name: "container_picker", drive: func(t *testing.T, m tui.Model) tui.Model {
			m, _ = tuitest.Drive(t, m, views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerSidecar, "app"}, NextKind: "exec",
			})
			return m
		}},
	}
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		for _, tc := range cases {
			name := fmt.Sprintf("screen_%s_%dx%d", tc.name, size[0], size[1])
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), size[0], size[1])
				tuitest.Golden(t, name, onTerminal(tuitest.Screen(tc.drive(t, m)), size[0])+"\n")
			})
		}
	}
}
