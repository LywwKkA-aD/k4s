package overlay_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

type answer struct{ yes bool }

func TestLoginAsksAndPreviewsTheCommand(t *testing.T) {
	t.Parallel()

	run := kubectl.New("version")
	cases := []struct {
		key  string
		want *answer // nil: the overlay stays open
	}{
		{key: "enter", want: &answer{yes: true}},
		{key: "n", want: &answer{}},
		{key: "esc", want: &answer{}},
		{key: "y"},
		{key: "x"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			o := overlay.NewLogin("eks-prod", run, answer{yes: true}, answer{}, clk.Now)
			if cmd, ok := overlay.CommandOf(o); !ok || cmd.String() != "kubectl version" {
				t.Errorf("CommandOf = %q, %v", cmd.String(), ok)
			}
			if overlay.TakesText(o) {
				t.Error("the login confirm takes text, so y would not copy")
			}
			for _, want := range []string{"log in", "eks-prod", "log in in a terminal?", "Enter"} {
				if !strings.Contains(screen(o), want) {
					t.Errorf("view lacks %q:\n%s", want, screen(o))
				}
			}
			clk.Advance(overlay.Grace)
			o, out := feed(t, o, tuitest.Press(tc.key))
			if tc.want == nil {
				requireOpen(t, o)
				if _, ok := overlay.CommandOf(o); !ok {
					t.Error("the open overlay lost its command")
				}
				return
			}
			if o != nil || len(out) != 1 || out[0] != *tc.want {
				t.Fatalf("open=%v, messages %s, want %+v", o != nil, tuitest.Describe(out), *tc.want)
			}
		})
	}
}
