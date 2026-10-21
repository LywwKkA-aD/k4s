package events_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/events"
)

// TestListsAsKubectlEvents pins the table: kubectl events' columns,
// NAMESPACE first across all namespaces, the newest event on top.
// Regenerate with go test ./internal/tui/views/events -run Lists -update
func TestListsAsKubectlEvents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		kubectl   string
	}{
		{name: "events_all", kubectl: "kubectl events -A"},
		{name: "events_demo", namespace: demo, kubectl: "kubectl events -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := mounted(t, client(t), tc.namespace)
			tuitest.Golden(t, tc.name, strings.Join(screenLines(m), "\n")+"\n")
			if got := m.Command(); got.String() != tc.kubectl || got.Approx {
				t.Errorf("Command = %q (approx %v), want %q", got.String(), got.Approx, tc.kubectl)
			}
			if m.Title() != "events" {
				t.Errorf("Title = %q", m.Title())
			}
		})
	}
}

// TestLastSeenFitsALongSeries: the widest LAST SEEN kubectl prints for a
// repeated event is not cut.
func TestLastSeenFitsALongSeries(t *testing.T) {
	t.Parallel()

	long := ev(demo, "long.1", "Warning", "BackOff", "again", pod(demo, "web-1"), 1234, 51*time.Hour, 59*time.Minute)
	m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(long)), "")
	if got := rows(m); len(got) != 1 || !strings.Contains(got[0], "59m (x1234 over 2d3h)") {
		t.Errorf("row:\n%s", strings.Join(got, "\n"))
	}
}

// TestWarningsOnly: W keeps the Warning events and says so in the
// command; W again brings the rest back.
func TestWarningsOnly(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t), "")
	if got := helpLabel(t, m, "W"); got != "warnings only" {
		t.Errorf("W label = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("W"))
	got := rows(m)
	if len(got) != 2 || !strings.Contains(got[0], "BackOff") || !strings.Contains(got[1], "NodeNotReady") {
		t.Errorf("warnings only:\n%s", strings.Join(got, "\n"))
	}
	if c := m.Command().String(); c != "kubectl events -A --types=Warning" {
		t.Errorf("Command = %q", c)
	}
	if got := helpLabel(t, m, "W"); got != "all types" {
		t.Errorf("W label with warnings only = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("W"))
	if n := len(rows(m)); n != 5 {
		t.Errorf("W again shows %d rows, want 5", n)
	}
}

func TestFilterAndBack(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t), "")
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("helm")...)
	if got := rows(m); len(got) != 1 || !strings.Contains(got[0], "HelmChart/traefik") {
		t.Fatalf("filter helm:\n%s", strings.Join(got, "\n"))
	}
	next, _, used := m.Back()
	if !used {
		t.Fatal("Esc with a committed filter went back")
	}
	if n := len(rows(next.(events.Model))); n != 5 {
		t.Errorf("after Esc: %d rows, want 5", n)
	}
	cleared := next.(events.Model)
	if _, _, used := cleared.Back(); used {
		t.Error("Esc without a filter did not go back")
	}
	cleared, _ = tuitest.Drive(t, cleared, tuitest.FilterBy("nothing-matches")...)
	if !strings.Contains(tuitest.Screen(cleared), "no events match /nothing-matches") {
		t.Errorf("empty filter result:\n%s", tuitest.Screen(cleared))
	}
}

// TestEnterDescribesTheObject: Enter opens describe of the object the
// event is about, under the name kubectl takes for its kind.
func TestEnterDescribesTheObject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		row  int
		want views.DescribeRequestMsg
	}{
		{row: 0, want: views.DescribeRequestMsg{Kind: "pod", Namespace: demo, Name: "web-1"}},
		{row: 2, want: views.DescribeRequestMsg{Kind: "deployment", Namespace: demo, Name: "web"}},
		{row: 3, want: views.DescribeRequestMsg{Kind: "node", Name: "node-1"}},
		{row: 4, want: views.DescribeRequestMsg{Kind: "helmchart.helm.cattle.io", Namespace: "kube-system", Name: "traefik"}},
	}
	for _, tc := range cases {
		t.Run(tc.want.Kind, func(t *testing.T) {
			t.Parallel()
			m, _ := mounted(t, client(t), "")
			for range tc.row {
				m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
			}
			_, out := tuitest.Drive(t, m, tuitest.Enter())
			got, ok := tuitest.Find[views.DescribeRequestMsg](out)
			if !ok || got != tc.want {
				t.Errorf("Enter on row %d asked %+v, want %+v:\n%s", tc.row, got, tc.want, tuitest.Describe(out))
			}
		})
	}
}

// TestEnterRefusesWhatKubectlWouldMisread: anyone who may create events
// writes involvedObject, so a name that reads as a flag or a kind that is
// no kind never reaches describe.
func TestEnterRefusesWhatKubectlWouldMisread(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		obj  runtime.Object
	}{
		{name: "flag name", obj: bad("--all", "Pod", "v1", demo)},
		{name: "bad kind", obj: bad("web-1", "Pod;rm", "v1", demo)},
		{name: "bad group", obj: bad("web-1", "Thing", "-x/v1", demo)},
		{name: "bad namespace", obj: bad("web-1", "Pod", "v1", "Demo NS")},
		{name: "no object", obj: bad("", "", "", "")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(tc.obj)), demo)
			_, out := tuitest.Drive(t, m, tuitest.Enter())
			if _, ok := tuitest.Find[views.DescribeRequestMsg](out); ok {
				t.Fatalf("Enter described a bad object:\n%s", tuitest.Describe(out))
			}
			requireStatus(t, out, status.Warn, "event odd.1 names no object kubectl can describe")
		})
	}
}

func bad(name, kind, apiVersion, ns string) runtime.Object {
	e := ev(demo, "odd.1", "Normal", "Odd", "odd", pod(ns, name), 1, time.Hour, time.Hour)
	e.InvolvedObject.Kind, e.InvolvedObject.APIVersion = kind, apiVersion
	return e
}

// TestSelectedIsTheEvent: o opens the YAML of the event itself, where its
// whole message is.
func TestSelectedIsTheEvent(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, client(t), "")
	var sel views.Selection = m
	got, ok := sel.Selected()
	want := views.ResourceRef{Kind: "event", Namespace: demo, Name: evBackOff}
	if !ok || got != want {
		t.Errorf("Selected = %+v, %v; want %+v", got, ok, want)
	}
	if _, ok := events.New(nil, "").Selected(); ok {
		t.Error("a view without rows selected something")
	}
}

// TestClusterTextIsInert: reason, object and message come from whoever
// wrote the event; no control sequence reaches the terminal and a
// multi-line message stays on its row.
func TestClusterTextIsInert(t *testing.T) {
	t.Parallel()

	evil := ev(demo, "evil.1", "Warning", "Bad\x1b[2JReason", "line one\nline two \x1b]52;c;cm0gLXJmIH4=\x07done",
		pod(demo, "web\x1b[31m-1"), 1, time.Hour, time.Hour)
	m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(evil)), demo)
	frame := tuitest.Frame(m)
	for _, b := range []string{"\x1b]", "\x07", "\x1b[2J", "\x1b[31m"} {
		if strings.Contains(frame, b) {
			t.Errorf("the frame carries %q", b)
		}
	}
	got := rows(m)
	if len(got) != 1 || !strings.Contains(got[0], "line one line two done") || !strings.Contains(got[0], "BadReason") {
		t.Errorf("row:\n%s", strings.Join(got, "\n"))
	}
}

// TestNewestRowsOnly: the table holds the newest 2000 events and says
// how many there are; the command is approximate then.
func TestNewestRowsOnly(t *testing.T) {
	t.Parallel()

	objs := make([]runtime.Object, 0, 2100)
	for i := range 2100 {
		objs = append(objs, ev(demo, fmt.Sprintf("e-%04d", i), "Normal", "Pulled", "pulled", pod(demo, "web-1"), 1, time.Hour, time.Duration(i+10)*time.Minute))
	}
	m, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(objs...)), demo)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "showing newest 2000 of 2100 events") {
		t.Errorf("no cap note:\n%s", screen)
	}
	if !m.Command().Approx {
		t.Error("a capped list gives an exact command")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("G"))
	if ref, _ := m.Selected(); ref.Name != "e-1999" {
		t.Errorf("last row = %q, want e-1999", ref.Name)
	}
}

func TestWithoutClient(t *testing.T) {
	t.Parallel()

	m, out := mounted(t, nil, "")
	if len(out) != 0 {
		t.Errorf("a view without a client did something:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "no kubeconfig") {
		t.Errorf("screen = %q", tuitest.Screen(m))
	}
	for _, k := range []string{"enter", "w", "r"} {
		_, out := tuitest.Drive(t, m, tuitest.Press(k))
		requireStatus(t, out, status.Warn, "no kubeconfig")
	}
	if m.CapturesKeys() || m.Close() != nil {
		t.Error("an idle view captures keys or fails to close")
	}
}

func TestEnterBeforeTheListSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := clocked(client(t), demo)
	_, out := tuitest.Step(t, m, tuitest.Enter())
	requireStatus(t, out, status.Info, "events are still loading")

	empty, _ := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()), demo)
	if !strings.Contains(tuitest.Screen(empty), "no events") {
		t.Errorf("empty list:\n%s", tuitest.Screen(empty))
	}
	_, out = tuitest.Drive(t, empty, tuitest.Enter())
	requireStatus(t, out, status.Warn, "no event selected")
}
