package tui_test

import (
	"fmt"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestHelpKeepsItsSectionsOnSmallTerminals: the popup is cut to the
// body, so a row added to the global keys costs the sections under them.
// With o listed, the view's section still starts on a 60x20 terminal and
// the dashboard's navigation keys still show on an 80x24 one. The
// dashboard lists five keys of its own, so its commands start from 30
// rows.
func TestHelpKeepsItsSectionsOnSmallTerminals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		width, height int
		wants         []string
	}{
		{width: 60, height: 20, wants: []string{"o       YAML of the selected object", "view: dashboard"}},
		{width: 80, height: 24, wants: []string{"o       YAML of the selected object", "view: dashboard", "↑↓       row"}},
		{width: 80, height: 30, wants: []string{"o       YAML of the selected object", "view: dashboard", "r        refresh", "commands"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil, instantFrames()), tc.width, tc.height)
			m, _ = tuitest.Drive(t, m, tuitest.Press("?"))
			requireScreen(t, m, tc.wants...)
		})
	}
}
