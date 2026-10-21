package top_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func header(m tui.Model) string {
	first, _, _ := strings.Cut(tuitest.Screen(m), "\n")
	return first
}

// TestEscBringsTopBackAsLeft: nodes mode, memory sort and a paused watch
// survive a trip to another view and back.
func TestEscBringsTopBackAsLeft(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(tuitest.Colon("top"), tuitest.Keys("n", "s", "w"))...)
	before := tuitest.Footer(m)
	tuitest.Golden(t, "footer_top_left", before+"\n")

	m, _ = tuitest.Drive(t, m, tuitest.Colon("pods")...)
	if h := header(m); tuitest.HeaderView(m) != "pods" {
		t.Fatalf(":pods: header %q", h)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if h := header(m); tuitest.HeaderView(m) != "top" {
		t.Fatalf("Esc from pods: header %q", h)
	}
	if after := tuitest.Footer(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, tuitest.NodeName) || strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("nodes mode not back:\n%s", screen)
	}
}
