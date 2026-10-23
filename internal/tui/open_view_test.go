package tui_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestOpenView: a view asks for a list as the command bar would open it:
// a namespaced list in the namespace asked for, in the state asked for,
// a cluster-wide one as it is; Esc goes back to the view that asked.
func TestOpenView(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		msg     views.OpenViewMsg
		view    string
		ns      string
		command string
	}{
		{name: "pods in a namespace", msg: views.OpenViewMsg{Name: "pods", Namespace: "demo"}, view: "pods", ns: "demo", command: "kubectl get pods -n demo"},
		{name: "deployments everywhere", msg: views.OpenViewMsg{Name: "deployments"}, view: "deployments", ns: "ALL", command: "kubectl get deployments -A"},
		{
			name: "the warnings", msg: views.OpenViewMsg{Name: "events", State: views.State{Mode: views.ModeWarnings}},
			view: "events", ns: "ALL", command: "kubectl events -A --types=Warning",
		},
		{
			name: "top by memory", msg: views.OpenViewMsg{Name: "top", Namespace: "demo", State: views.State{Sort: views.SortMemory}},
			view: "top", ns: "demo", command: "kubectl top pods -n demo --sort-by=memory",
		},
		{name: "nodes keep the namespace", msg: views.OpenViewMsg{Name: "nodes", Namespace: "demo"}, view: "nodes", ns: "ALL", command: "kubectl get nodes -o wide"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.msg)
			requireView(t, m, tc.view)
			if got := tuitest.HeaderNamespace(m); got != tc.ns {
				t.Errorf("header namespace = %q, want %q", got, tc.ns)
			}
			if got := commandRow(m); got != tc.command {
				t.Errorf("command row = %q, want %q", got, tc.command)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			requireView(t, m, "dashboard")
		})
	}
}

// TestOpenViewOfNoView: a name that is no list says so and stays.
func TestOpenViewOfNoView(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.OpenViewMsg{Name: "secrets"})
	requireView(t, m, "dashboard")
	requireScreen(t, m, "no k4s view for secrets")
}

// TestOpenViewRefusesAnInvalidNamespace: the namespace comes from a row
// the cluster named, past command.Parse; one kubectl would not take opens
// nothing and says so.
func TestOpenViewRefusesAnInvalidNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, namespace string }{
		{name: "reads as a flag", namespace: "--server=https://evil"},
		{name: "upper case", namespace: "Demo"},
		{name: "escape sequence", namespace: "a\x1b[2Jb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.OpenViewMsg{Name: "pods", Namespace: tc.namespace})
			requireView(t, m, "dashboard")
			requireScreen(t, m, "no k4s view for pods: invalid name: namespace")
			if got := tuitest.HeaderNamespace(m); got != "ALL" {
				t.Errorf("header namespace = %q, want ALL", got)
			}
		})
	}
}

// TestDashboardEnterOpensThePanelsList: Enter on a dashboard panel opens
// the k4s list its command stands for; Esc comes back to the dashboard.
func TestDashboardEnterOpensThePanelsList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		panel, view, command string
	}{
		{panel: "3", view: "pods", command: "kubectl get pods -A"},
		{panel: "5", view: "events", command: "kubectl events -A --types=Warning"},
		{panel: "2", view: "nodes", command: "kubectl get nodes -o wide"},
	}
	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Press(tc.panel), tuitest.Enter())
			requireView(t, m, tc.view)
			if got := commandRow(m); got != tc.command {
				t.Errorf("command row = %q, want %q", got, tc.command)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			requireView(t, m, "dashboard")
		})
	}
}
