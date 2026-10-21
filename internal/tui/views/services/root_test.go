package services_test

// Black-box tests through the root model: Esc from the logs view rebuilds
// the services view, and these check it comes back as it was left.

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The root is wide enough for the whole hint row, w label included.
const (
	rootW = 200
	rootH = 40
)

func startedRoot(t *testing.T, c *k8s.Client) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c.WithNamespace(shop), nil), rootW, rootH)
	return m
}

func rootLines(m tui.Model) []string {
	return strings.Split(tuitest.Screen(m), "\n")
}

func rootView(m tui.Model) string {
	_, after, _ := strings.Cut(rootLines(m)[0], "view: ")
	return strings.TrimSpace(after)
}

func requireRootView(t *testing.T, m tui.Model, want string) {
	t.Helper()
	if got := rootView(m); got != want {
		t.Fatalf("view = %q, want %q\n%s", got, want, tuitest.Screen(m))
	}
}

// rootDescribes reports which service Enter describes in the root.
func rootDescribes(t *testing.T, m tui.Model) string {
	t.Helper()
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	got, ok := tuitest.Find[views.DescribeRequestMsg](out)
	if !ok {
		t.Fatalf("enter described nothing:\n%s", tuitest.Describe(out))
	}
	return views.RowKey(got.Namespace, got.Name)
}

// toLogsOfThirdMatch opens services, commits "pay", puts the cursor on
// the third match (pay-web) and runs the extra keys there. It returns the
// root on services and on the logs view of pay-web.
func toLogsOfThirdMatch(t *testing.T, c *k8s.Client, extra ...tea.Msg) (onList, onLogs tui.Model) {
	t.Helper()
	keys := slices.Concat(
		[]tea.Msg{tuitest.Press(":"), tuitest.Type("svc"), tuitest.Enter()},
		tuitest.FilterBy("pay"),
		tuitest.Keys("down", "down"),
		extra,
	)
	onList, _ = tuitest.Drive(t, startedRoot(t, c), keys...)
	requireRootView(t, onList, "services")
	onLogs, out := tuitest.Drive(t, onList, tuitest.Press("l"), tuitest.Enter())
	req, ok := tuitest.Find[views.LogsRequestMsg](out)
	if !ok || req.Target.Name != "pay-web" {
		t.Fatalf("want logs of pay-web:\n%s", tuitest.Describe(out))
	}
	return onList, onLogs
}

// TestEscFromLogsBringsTheListBack (M3.9): filter "pay", cursor on the
// third match, l, Esc: the same filter, row and watch, and the same
// footer before and after.
func TestEscFromLogsBringsTheListBack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		extra  []tea.Msg
		golden string
	}{
		{name: "watching", golden: "root_footer_pay_watching"},
		{name: "paused", extra: tuitest.Keys("w"), golden: "root_footer_pay_paused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			onList, onLogs := toLogsOfThirdMatch(t, payClient(t), tc.extra...)
			before := tuitest.Footer(onList)
			tuitest.Golden(t, tc.golden, before+"\n")

			back, _ := tuitest.Drive(t, onLogs, tuitest.Esc())
			requireRootView(t, back, "services")
			if after := tuitest.Footer(back); after != before {
				t.Fatalf("footer after Esc:\n%s\nwant:\n%s", after, before)
			}
			if !strings.Contains(tuitest.Screen(back), "/ pay") {
				t.Fatalf("the filter is gone:\n%s", tuitest.Screen(back))
			}
			if got := rootDescribes(t, back); got != shop+"/pay-web" {
				t.Fatalf("enter describes %q, want %s/pay-web", got, shop)
			}
		})
	}
}

// TestEscAfterTheRowWasDeleted: the service went away while its logs were
// open; Esc lands on the row now at its index and says so.
func TestEscAfterTheRowWasDeleted(t *testing.T) {
	t.Parallel()

	c := payClient(t)
	_, onLogs := toLogsOfThirdMatch(t, c)
	deleteService(t, c, shop, "pay-web")
	back, _ := tuitest.Drive(t, onLogs, tuitest.Esc())
	requireRootView(t, back, "services")
	if footer := tuitest.Footer(back); !strings.Contains(footer, "service pay-web is gone") {
		t.Fatalf("footer lacks the status:\n%s", footer)
	}
	if got := rootDescribes(t, back); got != shop+"/payments" {
		t.Fatalf("enter describes %q, want %s/payments", got, shop)
	}
}

// TestEscClearsTheFilterBeforeLeaving: the first Esc clears a committed
// filter and stays; the cursor keeps its row.
func TestEscClearsTheFilterBeforeLeaving(t *testing.T) {
	t.Parallel()

	onList, _ := toLogsOfThirdMatch(t, payClient(t))
	cleared, _ := tuitest.Drive(t, onList, tuitest.Esc())
	requireRootView(t, cleared, "services")
	if strings.Contains(tuitest.Screen(cleared), "/ pay") {
		t.Fatalf("the filter is still up:\n%s", tuitest.Screen(cleared))
	}
	if got := rootDescribes(t, cleared); got != shop+"/pay-web" {
		t.Fatalf("enter describes %q, want the same row", got)
	}
	left, _ := tuitest.Drive(t, cleared, tuitest.Esc())
	requireRootView(t, left, "dashboard")
}

// TestLogsOnTheKubernetesServiceSaysSo (M3.2): the footer explains why l
// did nothing.
func TestLogsOnTheKubernetesServiceSaysSo(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(shop), tuitest.Service(shop, "kubernetes", 443, nil)))
	keys := slices.Concat([]tea.Msg{tuitest.Press(":"), tuitest.Type("svc"), tuitest.Enter()}, tuitest.Keys("l"))
	m, _ := tuitest.Drive(t, startedRoot(t, c), keys...)
	if footer := tuitest.Footer(m); !strings.Contains(footer, "service kubernetes has no selector") {
		t.Fatalf("footer lacks the status:\n%s", footer)
	}
}
