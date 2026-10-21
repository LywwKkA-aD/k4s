package nodes_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/nodes"
)

// TestListsAsKubectlGetNodesWide pins the table: roles of a k3s server
// with embedded etcd, a cordoned agent, kubelets of two versions, OS/ARCH
// and the taint count. Regenerate with
//
//	go test ./internal/tui/views/nodes -run Lists -update
func TestListsAsKubectlGetNodesWide(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t))
	tuitest.Golden(t, "nodes", strings.Join(screenLines(m), "\n")+"\n")
	if c := m.Command(); c.String() != "kubectl get nodes -o wide" || c.Approx {
		t.Errorf("Command = %q (approx %v)", c.String(), c.Approx)
	}
	if m.Title() != "nodes" {
		t.Errorf("Title = %q", m.Title())
	}
	got := rows(m)
	for i, want := range [][]string{
		{nodeServer, "Ready", "control-plane,etcd,master", "v1.36.5+k3s1", "10.0.0.2", "linux/amd64", "1"},
		{nodeAgent, "Ready", "<none>", "v1.36.5+k3s1", "10.0.0.11", "linux/arm64", "0"},
		{nodeOld, "Ready,SchedulingDisabled", "<none>", "v1.35.2+k3s1", "10.0.0.12", "linux/arm", "1"},
	} {
		if fields := strings.Fields(got[i]); !slices.Equal(slices.Delete(fields, 3, 4), want) {
			t.Errorf("row %d = %q, want %q around the age", i, got[i], want)
		}
	}
}

func TestHelpLabelsFollowTheBindings(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t))
	got := make([]string, 0, len(m.Help()))
	for _, b := range m.Help() {
		got = append(got, b.Help().Key+" "+b.Help().Desc)
	}
	want := []string{"enter describe", "/ filter", "w watch 5s", "r refresh"}
	if !slices.Equal(got, want) {
		t.Errorf("help = %q, want %q", got, want)
	}
}

func TestEnterDescribesTheNode(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t))
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	got, ok := tuitest.Find[views.DescribeRequestMsg](out)
	if want := (views.DescribeRequestMsg{Kind: "node", Name: nodeAgent}); !ok || got != want {
		t.Errorf("Enter asked %+v, want %+v", got, want)
	}
	var sel views.Selection = m
	if ref, ok := sel.Selected(); !ok || ref != (views.ResourceRef{Kind: "node", Name: nodeAgent}) {
		t.Errorf("Selected = %+v, %v", ref, ok)
	}
}

func TestFilterAndBack(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t))
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("disabled")...)
	if got := rows(m); len(got) != 1 || !strings.Contains(got[0], nodeOld) {
		t.Fatalf("filter disabled:\n%s", strings.Join(got, "\n"))
	}
	next, _, used := m.Back()
	if !used || len(rows(next.(nodes.Model))) != 3 {
		t.Fatalf("Esc did not clear the filter")
	}
	cleared, _ := tuitest.Drive(t, next.(nodes.Model), tuitest.FilterBy("nope")...)
	if !strings.Contains(tuitest.Screen(cleared), "no nodes match /nope") {
		t.Errorf("empty filter:\n%s", tuitest.Screen(cleared))
	}
	if _, _, used := next.(nodes.Model).Back(); used {
		t.Error("Esc without a filter did not go back")
	}
}

// TestClusterTextIsInert: the kubelet reports its own version and OS; a
// node that lies cannot drive the terminal.
func TestClusterTextIsInert(t *testing.T) {
	t.Parallel()

	evil := node("evil", "10.0.0.66", "v1.36.5\x1b]52;c;cm0gLXJmIH4=\x07\tk3s", "arm64\x1b[2J", nil, 0)
	m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(evil)))
	frame := tuitest.Frame(m)
	for _, b := range []string{"\x1b]", "\x07", "\x1b[2J", "\t"} {
		if strings.Contains(frame, b) {
			t.Errorf("the frame carries %q", b)
		}
	}
	if got := rows(m); len(got) != 1 || !strings.Contains(got[0], "linux/arm64") || !strings.Contains(got[0], "v1.36.5 k3s") {
		t.Errorf("row:\n%s", strings.Join(got, "\n"))
	}
}

func TestWithoutClient(t *testing.T) {
	t.Parallel()

	m, out := mounted(t, nil)
	if len(out) != 0 || !strings.Contains(tuitest.Screen(m), "no kubeconfig") {
		t.Errorf("a view without a client: %q\n%s", tuitest.Screen(m), tuitest.Describe(out))
	}
	for _, k := range []string{"enter", "w", "r"} {
		_, out := tuitest.Drive(t, m, tuitest.Press(k))
		requireStatus(t, out, status.Warn, "no kubeconfig")
	}
	if m.CapturesKeys() || m.Close() != nil {
		t.Error("an idle view captures keys or fails to close")
	}
	if _, ok := m.Selected(); ok {
		t.Error("a view without rows selected something")
	}
}

func TestEnterBeforeTheListSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := clocked(client(t))
	_, out := tuitest.Step(t, m, tuitest.Enter())
	requireStatus(t, out, status.Info, "nodes are still loading")

	empty, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()))
	if !strings.Contains(tuitest.Screen(empty), "no nodes") {
		t.Errorf("empty list:\n%s", tuitest.Screen(empty))
	}
	_, out = tuitest.Drive(t, empty, tuitest.Enter())
	requireStatus(t, out, status.Warn, "no node selected")
}

// TestForbiddenSaysHowToCheck (M4.3): listing nodes needs a ClusterRole;
// a namespace-scoped user is told what to ask for.
func TestForbiddenSaysHowToCheck(t *testing.T) {
	t.Parallel()

	c, cs := k8stest.NewFakeClient(fixture()...)
	k8stest.RestrictToNamespace(cs, "demo")
	m, out := mounted(t, c)
	want := "forbidden: list nodes · kubectl auth can-i list nodes"
	requireStatus(t, out, status.Error, want)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "nodes unavailable: "+want) || !strings.Contains(screen, "is forbidden") {
		t.Errorf("body:\n%s", screen)
	}
	_, out = tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.Warn, "nodes unavailable, press r")
}

// TestPollsReadTheWatchCacheAndRefreshReadsThrough (M4.4).
func TestPollsReadTheWatchCacheAndRefreshReadsThrough(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, clk := clocked(client(t, log.reactor()))
	m, _ = tuitest.Mount(t, m, width, height)
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	m, _ = tuitest.Drive(t, m, tick)
	_, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got, want := log.seen(), []string{"0", "0", ""}; !slices.Equal(got, want) {
		t.Fatalf("list resource versions = %q, want %q", got, want)
	}
	busy, _ := clocked(client(t))
	_, out := tuitest.Step(t, busy, tuitest.Press("r"))
	requireStatus(t, out, status.Info, "refresh already running")
}

func TestWatchToggleStopsTheTicks(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, clk := clocked(client(t, log.reactor()))
	m, _ = tuitest.Mount(t, m, width, height)
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := helpLabel(t, m, "w"); got != "paused" {
		t.Fatalf("w label after w = %q, want paused", got)
	}
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := log.seen(); len(got) != 1 {
		t.Fatalf("a paused view listed on its old tick: %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if tick, ok := clk.Fire(); !ok {
		t.Fatal("w back on armed no tick")
	} else {
		_, _ = tuitest.Drive(t, m, tick)
	}
	if got := log.seen(); len(got) != 2 {
		t.Fatalf("lists after watch came back = %q, want 2", got)
	}
}

// TestFailedRefreshKeepsTheRows: the rows of the last good list stay
// under a stale banner, and the error is said once.
func TestFailedRefreshKeepsTheRows(t *testing.T) {
	t.Parallel()

	var down tuitest.Outage
	m, clk := clocked(client(t, down.Lists("nodes", errors.New("connection refused"))))
	m, _ = tuitest.Mount(t, m, width, height)
	down.Down()
	tick, _ := clk.Fire()
	m, out := tuitest.Drive(t, m, tick)
	requireStatus(t, out, status.Error, "list nodes: connection refused")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "stale: connection refused") || !strings.Contains(screen, nodeServer) {
		t.Errorf("stale view:\n%s", screen)
	}
	tick, _ = clk.Fire()
	if _, out = tuitest.Drive(t, m, tick); len(tuitest.All[views.StatusMsg](out)) != 0 {
		t.Errorf("the same error was said twice:\n%s", tuitest.Describe(out))
	}
}

// TestTheBodySaysWhatTheStatusRowSaid: a first list that fails shows the
// status row's own text in the body, however the error is classified.
func TestTheBodySaysWhatTheStatusRowSaid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "API slow: list nodes timed out · r retries"},
		{name: "other", err: errors.New("etcd\x1b[2J down\nsecond line"), want: "list nodes: etcd down"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failing := tuitest.WithReactor("list", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, tc.err
			})
			m, out := mounted(t, client(t, failing))
			requireStatus(t, out, status.Error, tc.want)
			if lines := screenLines(m); !slices.Contains(lines, "nodes unavailable: "+tc.want) {
				t.Errorf("body:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

// TestStateRoundTrip: the filter, the row under the cursor and a paused
// watch come back as they were left; a node that left says so.
func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	c := client(t)
	m, _ := mounted(t, c)
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("pi")...)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"), tuitest.Press("w"))
	st := m.State()
	want := views.State{Filter: "pi", Selected: nodeOld, Cursor: 1, Paused: true}
	if st != want {
		t.Fatalf("State = %+v, want %+v", st, want)
	}
	back, _ := restore(t, c, st)
	if got := back.State(); got != want {
		t.Errorf("restored State = %+v, want %+v", got, want)
	}

	if err := c.Clientset.CoreV1().Nodes().Delete(context.Background(), nodeOld, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete node: %v", err)
	}
	gone, out := restore(t, c, st)
	requireStatus(t, out, status.Info, "node "+nodeOld+" is gone")
	if ref, _ := gone.Selected(); ref.Name != nodeAgent {
		t.Errorf("cursor on %q, want the last row left, %q", ref.Name, nodeAgent)
	}
}

func restore(t *testing.T, c *k8s.Client, st views.State) (nodes.Model, []tea.Msg) {
	t.Helper()
	fresh, _ := clocked(c)
	next, cmd := fresh.Restore(st)
	if screen := tuitest.Screen(next); st.Filter != "" && !strings.Contains(screen, st.Filter) {
		t.Errorf("first restored frame lacks the filter:\n%s", screen)
	}
	m, out := tuitest.Mount(t, next.(nodes.Model), width, height)
	return m, append(tuitest.Collect(t, cmd), out...)
}
