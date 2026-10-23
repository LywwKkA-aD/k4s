package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

const (
	verdictCmd = "≈ kubectl get nodes,pods,deploy,sts,ds -A"
	readyzCmd  = "kubectl get '--raw=/readyz?verbose'"
	nodesCmd   = "kubectl get nodes -o wide"
	podsCmd    = "kubectl get pods -A"
	firstPod   = "kubectl describe pod payments-worker-6c9f7d8b5-xk2lp -n shop"
)

// TestFocusKeys walks the focus with the spec's keys (§5) and reads the
// footer command it lands on.
func TestFocusKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "the banner first", want: verdictCmd},
		{name: "tab to panel 1", keys: []string{"tab"}, want: readyzCmd},
		{name: "tab twice", keys: []string{"tab", "tab"}, want: nodesCmd},
		{name: "shift+tab wraps to the last panel", keys: []string{"shift+tab"}, want: "kubectl top pods -A --sort-by=cpu"},
		{name: "jump", keys: []string{"3"}, want: podsCmd},
		{name: "jump back to the banner", keys: []string{"3", "0"}, want: verdictCmd},
		{name: "down onto the first row", keys: []string{"3", "down"}, want: firstPod},
		{name: "j and k", keys: []string{"3", "j", "j", "k"}, want: firstPod},
		{name: "up off the first row", keys: []string{"3", "down", "up"}, want: podsCmd},
		{name: "right to the neighbour", keys: []string{"3", "right"}, want: "kubectl get deploy,sts,ds -A"},
		{name: "left to the neighbour", keys: []string{"2", "left"}, want: readyzCmd},
		{name: "a cluster row", keys: []string{"1", "down", "down"}, want: "kubectl version"},
		{name: "a node row", keys: []string{"2", "down"}, want: "kubectl describe node k3s-cp-1"},
		{name: "the history row is approximate", keys: []string{"2", "down", "down", "down", "down"}, want: "≈ kubectl top nodes"},
		{name: "h does not move the focus", keys: []string{"3", "h"}, want: podsCmd},
		{name: "l does not move the focus", keys: []string{"3", "l"}, want: podsCmd},
		{name: "enter on the banner goes to the worst panel", keys: []string{"enter"}, want: podsCmd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), tt.keys...)
			if got := command(m); got != tt.want {
				t.Fatalf("command after %v = %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

// TestChartRowOnlyWhereDrawn: the history chart is a cursor stop only
// where the nodes panel has room to draw it. Too short, down stays on the
// last node; shrinking the terminal under the cursor moves it there.
func TestChartRowOnlyWhereDrawn(t *testing.T) {
	t.Parallel()

	const lastNode = "kubectl describe node k3s-agent-2"
	short, _ := press(t, mount(t, newFeed(busyK3s), 80, 15), "2", "down", "down", "down", "down", "down")
	if got := command(short); got != lastNode {
		t.Fatalf("down past the last node of a short panel = %q, want %q", got, lastNode)
	}

	wide, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "2", "down", "down", "down", "down")
	if got := command(wide); got != "≈ kubectl top nodes" {
		t.Fatalf("down onto the drawn chart = %q", got)
	}
	shrunk, _ := tuitest.Drive(t, wide, tuitest.Resize(80, 15))
	if got := command(shrunk); got != lastNode {
		t.Fatalf("after the chart left the screen = %q, want %q", got, lastNode)
	}
}

// TestEscSteps backs out one level per Esc: row, panel, banner; on the
// banner Esc is the root's.
func TestEscSteps(t *testing.T) {
	t.Parallel()

	m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3", "down")
	for _, want := range []string{podsCmd, verdictCmd} {
		next, _, used := m.Back()
		if !used {
			t.Fatalf("Back was not used on the way to %q", want)
		}
		if got := next.Command().Display(); got != want {
			t.Fatalf("after Esc = %q, want %q", got, want)
		}
		var ok bool
		if m, ok = next.(dashboard.Model); !ok {
			t.Fatalf("Back returned %T", next)
		}
	}
	if _, _, used := m.Back(); used {
		t.Fatal("Esc on the banner must be left to the root")
	}
}

func TestEnterDescribesAPod(t *testing.T) {
	t.Parallel()

	_, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3", "down", "enter")
	want := views.DescribeRequestMsg{Kind: "pod", Namespace: "shop", Name: "payments-worker-6c9f7d8b5-xk2lp"}
	if !tuitest.Contains(out, want) {
		t.Fatalf("enter on a pod row:\n%s", tuitest.Describe(out))
	}
}

func TestEnterWithoutAViewSaysSo(t *testing.T) {
	t.Parallel()

	_, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "1", "down", "enter")
	msg, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || !strings.Contains(msg.Text, "no k4s view") {
		t.Fatalf("enter without a view:\n%s", tuitest.Describe(out))
	}
}

// TestJumpToAMissingPanel: a panel this cluster does not have says why
// and leaves the focus where it was. (One this size leaves out opens
// zoomed: TestHiddenPanelOpensZoomed.)
func TestJumpToAMissingPanel(t *testing.T) {
	t.Parallel()

	m, out := press(t, mount(t, newFeed(nsScopedEKS), 120, 34), "6")
	msg, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || !strings.Contains(msg.Text, "6 k3s: not a k3s cluster") {
		t.Fatalf("6 on EKS:\n%s", tuitest.Describe(out))
	}
	if got := command(m); got != "≈ kubectl get nodes,pods,deploy,sts,ds -n shop" {
		t.Fatalf("focus moved to a missing panel: %q", got)
	}
}

func TestHintsFollowTheFocus(t *testing.T) {
	t.Parallel()

	m := mount(t, newFeed(busyK3s), wideW, wideH)
	if got := tuitest.HelpKeys(m); strings.Join(got, " ") != "tab/0-7 ←→ ↑↓ enter r w" {
		t.Fatalf("hints on the banner = %v", got)
	}
	m, _ = press(t, m, "1")
	if got := tuitest.HelpKeys(m); strings.Join(got, " ") != "tab/0-7 ←→ ↑↓ enter z r w" {
		t.Fatalf("hints on panel 1 = %v, want enter for :ns", got)
	}
	m, _ = press(t, m, "3", "down")
	if hint(m, "enter") != "describe" {
		t.Fatalf("enter hint on a pod row = %q, want describe", hint(m, "enter"))
	}
}

// TestEnterHintOnTheBannerNeedsItsPanel: Enter on the banner goes to the
// most severe panel, zoomed where the size leaves it out; the hint is
// left out only where there is no room to show it at all.
func TestEnterHintOnTheBannerNeedsItsPanel(t *testing.T) {
	t.Parallel()

	base := func() dash.Snapshot {
		s := busyK3s()
		s.Verdict.Focus = dash.PanelTopPods
		return s
	}
	if got := hint(mount(t, newFeed(base), wideW, wideH), "enter"); got != "go to top pods" {
		t.Fatalf("enter hint with top pods on screen = %q", got)
	}
	medium := mount(t, newFeed(base), 120, 34)
	if got := hint(medium, "enter"); got != "go to top pods" {
		t.Fatalf("enter hint with top pods left out = %q", got)
	}
	if m, _ := press(t, medium, "enter"); !strings.Contains(tuitest.Screen(m), "7 top pods") {
		t.Fatal("enter on the banner did not open top pods zoomed")
	}
	if got := hint(mount(t, newFeed(base), 38, 5), "enter"); got != "" {
		t.Fatalf("enter hint with no room for a panel = %q, want none", got)
	}
}

func TestHelpMapListsThePanels(t *testing.T) {
	t.Parallel()

	// Every panel the cluster has, also where the size leaves it out or a
	// zoom covers it: its number opens it.
	medium := mount(t, newFeed(busyK3s), 120, 34)
	zoomed, _ := press(t, medium, "3", "z")
	for _, v := range []dashboard.Model{mount(t, newFeed(busyK3s), wideW, wideH), medium, zoomed} {
		keys := make([]string, 0, len(v.HelpMap()))
		for _, b := range v.HelpMap() {
			keys = append(keys, b.Help().Key)
		}
		if strings.Join(keys, "") != "01234567" {
			t.Fatalf("map keys = %v, want 0 to 7", keys)
		}
	}
	m := mount(t, newFeed(busyK3s), wideW, wideH)
	if desc := m.HelpMap()[3].Help().Desc; desc != "pods · "+podsCmd {
		t.Fatalf("map entry 3 = %q", desc)
	}
}
