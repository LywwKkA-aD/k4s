package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// readyzCommand is panel 1's command: tab from the banner focuses it.
const readyzCommand = "kubectl get '--raw=/readyz?verbose'"

func TestDashboardMemorySurvivesGoingHome(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Press("tab"))
	if got := footer(m); !strings.Contains(got, readyzCommand) {
		t.Fatalf("tab did not focus the cluster panel, footer:\n%s", got)
	}
	m, _ = tuitest.Drive(t, m, colon("pods")...)
	requireView(t, m, "pods")

	// q goes home; the dashboard is back as it was before its first new
	// answer arrives: focus kept, data shown, nothing loading.
	home, _ := tuitest.Step(t, m, tuitest.Press("q"))
	requireView(t, home, "dashboard")
	refuteScreen(t, home, "loading")
	if got := footer(home); !strings.Contains(got, readyzCommand) {
		t.Errorf("focus was lost going home, footer:\n%s", got)
	}
}

func TestDashboardMemoryIsDroppedOnContextSwitch(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Press("tab"))
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "beta"})
	requireView(t, m, "dashboard")
	if got := footer(m); strings.Contains(got, readyzCommand) {
		t.Errorf("the new context kept the old dashboard's focus, footer:\n%s", got)
	}
}

// TestDashboardBodyCommandsCarryTheSession: after :ctx the banner's next
// line, pasted into a shell, reaches the context k4s switched to, not the
// kubeconfig's current-context.
func TestDashboardBodyCommandsCarryTheSession(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.ContextSelectedMsg{Name: "beta"})
	requireView(t, m, "dashboard")
	next := ""
	for _, l := range lines(m) {
		if strings.HasPrefix(strings.TrimSpace(l), "▸") {
			next = l
		}
	}
	if !strings.Contains(next, "kubectl --context beta ") {
		t.Fatalf("banner next line = %q, want it decorated with --context beta\n%s", next, tuitest.Screen(m))
	}
}

func TestDashboardFollowsTerminalFocus(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Blur())
	if got := footer(m); !strings.Contains(got, "w watch 15s") {
		t.Fatalf("blurred dashboard should poll at 15s, footer:\n%s", got)
	}

	// The blur is remembered across a visit to another view.
	m, _ = tuitest.Drive(t, m, colon("pods")...)
	m, _ = tuitest.Drive(t, m, tuitest.Press("q"))
	requireView(t, m, "dashboard")
	if got := footer(m); !strings.Contains(got, "w watch 15s") {
		t.Fatalf("a dashboard built while blurred should poll at 15s, footer:\n%s", got)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Focus())
	if got := footer(m); !strings.Contains(got, "w watch 5s") {
		t.Fatalf("focus regained should poll at 5s, footer:\n%s", got)
	}
}

func TestViewReportsFocus(t *testing.T) {
	t.Parallel()

	if !tui.New(nil, nil).View().ReportFocus {
		t.Fatal("the root view must ask the terminal to report focus")
	}
}

func TestDashboardFooterHasNoWatchSuffix(t *testing.T) {
	t.Parallel()

	m := started(t, tuitest.NewFakeClient(t))
	if got := footer(m); strings.Contains(got, "--watch") || strings.Contains(got, "get all") {
		t.Fatalf("footer = %q, want neither --watch nor the old get all", got)
	}
}

func TestHelpShowsTheDashboardMap(t *testing.T) {
	t.Parallel()

	tall, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Resize(screenW, 70), tuitest.Press("?"))
	requireScreen(t, tall, "dashboard map", "cluster · "+readyzCommand, ":forwards")

	// The popup does not scroll: where the map would push the commands
	// off screen, it is left out.
	short, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Press("?"))
	requireScreen(t, short, "view: dashboard", ":forwards")
}
