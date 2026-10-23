package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// Rows of the help popup that tell which of its pages is shown.
const (
	helpTitle    = "help   ? / Esc close"
	tmuxNote     = "in tmux: set -g set-clipboard on"
	yamlRow      = "o       YAML of the selected object"
	globalRow    = "global  q quit  ^c quit  esc back  : command  o yaml  y copy"
	globalRowOff = "global  q home  ^c quit  esc back  : command  o yaml  y copy"
	commandsRow  = ": home pods ns deploy svc events nodes ctx top pf"
	commandsFull = ":ns <name>"
	dashMap      = "dashboard map"
)

// TestHelpFitsTheBody: the popup does not scroll, so it is always whole
// on the body, its border and its title row with the close hint. Where
// the body is short it loses, in this order: the commands down to one
// row, the view's map, the tmux note, the global keys down to one row and
// the blank rows. The view's own keys stay; a body too short even then
// cuts them inside the border.
func TestHelpFitsTheBody(t *testing.T) {
	t.Parallel()

	dashboardKeys := []string{"view: dashboard", "tab/0-7  panel", "↑↓       row", "r        refresh", "w        5s"}
	podsKeys := []string{"view: pods", "describe", "port-forward", "/      filter", "r      refresh"}
	cases := []struct {
		view          string
		input         []tea.Msg
		width, height int
		wants, nots   []string
	}{
		{
			view: "dashboard", width: 80, height: 70,
			wants: append([]string{yamlRow, tmuxNote, dashMap, ":pods", commandsFull}, dashboardKeys...),
		},
		{
			view: "dashboard", width: 80, height: 46,
			wants: append([]string{yamlRow, tmuxNote, dashMap, commandsRow}, dashboardKeys...),
			nots:  []string{commandsFull},
		},
		{
			view: "dashboard", width: 80, height: 30,
			wants: append([]string{yamlRow, tmuxNote, commandsRow}, dashboardKeys...),
			nots:  []string{dashMap, commandsFull},
		},
		{
			view: "dashboard", width: 80, height: 24,
			wants: append([]string{globalRow, commandsRow}, dashboardKeys...),
			nots:  []string{yamlRow, tmuxNote, dashMap, commandsFull},
		},
		{
			view: "pods", input: colon("pods"), width: 80, height: 46,
			wants: append([]string{yamlRow, tmuxNote, ":pods", commandsFull}, podsKeys...),
		},
		{
			view: "pods", input: colon("pods"), width: 80, height: 30,
			wants: append([]string{yamlRow, commandsRow}, podsKeys...),
			nots:  []string{tmuxNote, commandsFull},
		},
		{
			view: "pods", input: colon("pods"), width: 80, height: 24,
			wants: append([]string{globalRowOff, commandsRow}, podsKeys...),
			nots:  []string{yamlRow, tmuxNote, commandsFull},
		},
		{
			// Seven keys of its own leave no room for the commands row:
			// the popup is cut inside its border.
			view: "pods", input: colon("pods"), width: 60, height: 20,
			wants: []string{"view: pods", "describe", "…"},
			nots:  []string{commandsRow},
		},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %dx%d", tc.view, tc.width, tc.height), func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil, instantFrames()), tc.width, tc.height)
			m, _ = tuitest.Drive(t, m, tc.input...)
			m, _ = tuitest.Drive(t, m, tuitest.Press("?"))
			requireView(t, m, tc.view)
			requireWholePopup(t, m)
			requireScreen(t, m, tc.wants...)
			refuteScreen(t, m, tc.nots...)
		})
	}
}

// requireWholePopup fails unless the popup's top border, its title row
// and its bottom border are all on the screen, in that order.
func requireWholePopup(t *testing.T, m tui.Model) {
	t.Helper()
	screen := tuitest.Screen(m)
	top, title, bottom := -1, -1, -1
	for i, l := range strings.Split(screen, "\n") {
		s := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(s, "╭") && strings.HasSuffix(s, "╮"):
			top = i
		case strings.Contains(s, helpTitle):
			title = i
		case strings.HasPrefix(s, "╰") && strings.HasSuffix(s, "╯"):
			bottom = i
		}
	}
	if top < 0 || title < top || bottom < title {
		t.Fatalf("the popup is not whole (top %d, title %d, bottom %d):\n%s", top, title, bottom, screen)
	}
}
