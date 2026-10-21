package top_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain keeps the root model these tests drive away from the
// developer's home: no saved port-forwards (which would open the restore
// popup) and no real kubeconfig.
func TestMain(m *testing.M) {
	os.Exit(isolated(m))
}

func isolated(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "k4s-top-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state")); err != nil {
		panic(err)
	}
	kubeconfig, err := tuitest.WriteKubeconfig(tmp, tuitest.FakeContext, tuitest.FakeContext)
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		panic(err)
	}
	return m.Run()
}

func colon(name string) []tea.Msg {
	return []tea.Msg{tuitest.Press(":"), tuitest.Type(name), tuitest.Enter()}
}

// rootFooter is the frame under the footer's rule: the hints or status row
// and the kubectl row.
func rootFooter(m tui.Model) string {
	lines := strings.Split(tuitest.Screen(m), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" && strings.Trim(s, "─") == "" {
			return tuitest.TrimLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

func header(m tui.Model) string {
	first, _, _ := strings.Cut(tuitest.Screen(m), "\n")
	return first
}

// TestEscBringsTopBackAsLeft: nodes mode, memory sort and a paused watch
// survive a trip to another view and back.
func TestEscBringsTopBackAsLeft(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(colon("top"), tuitest.Keys("n", "s", "w"))...)
	before := rootFooter(m)
	tuitest.Golden(t, "footer_top_left", before+"\n")

	m, _ = tuitest.Drive(t, m, colon("pods")...)
	if h := header(m); !strings.Contains(h, "view: pods") {
		t.Fatalf(":pods: header %q", h)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if h := header(m); !strings.Contains(h, "view: top") {
		t.Fatalf("Esc from pods: header %q", h)
	}
	if after := rootFooter(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, tuitest.NodeName) || strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("nodes mode not back:\n%s", screen)
	}
}
