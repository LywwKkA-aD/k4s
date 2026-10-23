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
// the commands still start on an 80x24 one, as before o was listed.
func TestHelpKeepsItsSectionsOnSmallTerminals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		width, height int
		wants         []string
	}{
		{width: 60, height: 20, wants: []string{"o       YAML of the selected object", "view: dashboard"}},
		{width: 80, height: 24, wants: []string{"o       YAML of the selected object", "view: dashboard", "w  watch", "commands"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), tc.width, tc.height)
			m, _ = tuitest.Drive(t, m, tuitest.Press("?"))
			requireScreen(t, m, tc.wants...)
		})
	}
}
