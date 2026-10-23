package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui"
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
			if got := tuitest.HeaderNamespace(m); got != "ALL" {
				t.Errorf("header namespace after Esc = %q, want the dashboard's ALL", got)
			}
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

// TestDashboardEnterOpensThePanelsList: Enter on a dashboard panel, or
// on a row that stands for a list, opens the k4s list its command stands
// for; Esc comes back to the dashboard as it was left: the namespace, the
// panel, the row and the zoom.
func TestDashboardEnterOpensThePanelsList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// keys put the cursor where Enter goes; edit fills in what the
		// fake cluster does not serve.
		keys              []string
		edit              func(dash.Snapshot) dash.Snapshot
		view, ns, command string
		zoomed            bool
	}{
		{name: "pods", keys: []string{"3"}, view: "pods", ns: "ALL", command: "kubectl get pods -A"},
		{name: "events", keys: []string{"5"}, view: "events", ns: "ALL", command: "kubectl events -A --types=Warning"},
		{name: "nodes", keys: []string{"2"}, view: "nodes", ns: "ALL", command: "kubectl get nodes -o wide"},
		{
			name: "k3s in kube-system", keys: []string{"6"}, edit: k3sPanel,
			view: "deployments", ns: "kube-system", command: "kubectl get deployments -n kube-system",
		},
		{
			name: "a row of a zoomed panel", keys: []string{"1", "down", "z"},
			view: "services", ns: "ALL", command: "kubectl get services -A", zoomed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t)
			opts := []tui.Option{instantFrames()}
			if tc.edit != nil {
				opts = append(opts, answeredWith(t, c, tc.edit))
			}
			// At wideW the hints row is whole: it names the enter, zoom and
			// watch state, and the command row the panel or row focused.
			m, _ := tuitest.Mount(t, tui.New(c, nil, opts...), wideW, screenH)
			m, _ = tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			left, ns := footer(m), tuitest.HeaderNamespace(m)
			if zoomed := !strings.Contains(tuitest.Screen(m), "2 nodes"); zoomed != tc.zoomed {
				t.Fatalf("zoomed before Enter = %v, want %v:\n%s", zoomed, tc.zoomed, tuitest.Screen(m))
			}

			m, _ = tuitest.Drive(t, m, tuitest.Enter())
			requireView(t, m, tc.view)
			if got := tuitest.HeaderNamespace(m); got != tc.ns {
				t.Errorf("header namespace = %q, want %q", got, tc.ns)
			}
			if got := commandRow(m); got != tc.command {
				t.Errorf("command row = %q, want %q", got, tc.command)
			}

			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			requireView(t, m, "dashboard")
			if got := tuitest.HeaderNamespace(m); got != ns {
				t.Errorf("header namespace after Esc = %q, want %q", got, ns)
			}
			if got := footer(m); got != left {
				t.Errorf("Esc came back elsewhere, footer:\n%s\nwant:\n%s", got, left)
			}
			if zoomed := !strings.Contains(tuitest.Screen(m), "2 nodes"); zoomed != tc.zoomed {
				t.Errorf("zoomed after Esc = %v, want %v:\n%s", zoomed, tc.zoomed, tuitest.Screen(m))
			}
		})
	}
}
