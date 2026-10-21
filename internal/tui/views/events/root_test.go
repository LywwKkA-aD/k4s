package events_test

// Black-box tests through the root model: :events and :ev open the view,
// Enter and o leave it for the event's object and the event's YAML, and
// Esc brings it back as it was left.

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

const (
	rootW = 200
	rootH = 40
)

func startedRoot(t *testing.T, c *k8s.Client) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil), rootW, rootH)
	return m
}

func rootView(m tui.Model) string {
	return tuitest.HeaderView(m)
}

func requireRootView(t *testing.T, m tui.Model, want string) {
	t.Helper()
	if got := rootView(m); got != want {
		t.Fatalf("view = %q, want %q\n%s", got, want, tuitest.Screen(m))
	}
}

// kubectlRow is the footer's command row.
func kubectlRow(m tui.Model) string {
	lines := strings.Split(tuitest.Footer(m), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func TestColonOpensTheEventsView(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"events", "ev", "event"} {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, startedRoot(t, client(t)), tuitest.Colon(alias)...)
			requireRootView(t, m, "events")
			if got := kubectlRow(m); got != "kubectl events -A" {
				t.Errorf("kubectl row = %q", got)
			}
			if !strings.Contains(tuitest.Screen(m), "Back-off restarting failed container") {
				t.Errorf("no events listed:\n%s", tuitest.Screen(m))
			}
		})
	}
}

// TestEscFromDescribeBringsEventsBack: W, a filter and the row under the
// cursor survive Enter (describe of the event's object) and Esc.
func TestEscFromDescribeBringsEventsBack(t *testing.T) {
	t.Parallel()

	keys := slices.Concat(tuitest.Colon("ev"), tuitest.Keys("W"), tuitest.FilterBy("web"))
	m, _ := tuitest.Drive(t, startedRoot(t, client(t)), keys...)
	before := tuitest.Footer(m)
	if got := kubectlRow(m); got != "kubectl events -A --types=Warning" {
		t.Fatalf("kubectl row = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if got := kubectlRow(m); got != "kubectl describe pod web-1 -n demo" {
		t.Errorf("Enter described %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireRootView(t, m, "events")
	if after := tuitest.Footer(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ web") || strings.Contains(screen, "Pulled") {
		t.Errorf("filter or W lost:\n%s", screen)
	}
}

// TestOOpensTheEventsYAML: o shows the event itself, whole message
// included.
func TestOOpensTheEventsYAML(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, startedRoot(t, client(t)), slices.Concat(tuitest.Colon("ev"), tuitest.Keys("o"))...)
	if got := kubectlRow(m); got != "kubectl get event "+evBackOff+" -n demo -o yaml" {
		t.Errorf("kubectl row = %q", got)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "kind: Event") || !strings.Contains(screen, "reason: BackOff") {
		t.Errorf("YAML:\n%s", screen)
	}
}

// TestNamespaceSwitchKeepsTheEventsView: :ns on the events view lists the
// same view in the new namespace, with W still on.
func TestNamespaceSwitchKeepsTheEventsView(t *testing.T) {
	t.Parallel()

	keys := slices.Concat(tuitest.Colon("ev"), tuitest.Keys("W"), []tea.Msg{tuitest.Press(":"), tuitest.Type("ns demo"), tuitest.Enter()})
	m, _ := tuitest.Drive(t, startedRoot(t, client(t)), keys...)
	requireRootView(t, m, "events")
	if got := kubectlRow(m); got != "kubectl events -n demo --types=Warning" {
		t.Errorf("kubectl row = %q", got)
	}
}
