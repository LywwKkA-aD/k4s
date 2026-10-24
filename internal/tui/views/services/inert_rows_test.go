package services_test

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostileWeb is the raw name of the one hostile service among clean ones.
// It sorts after them (U+202E is past ASCII), so it is never raw index 0.
var hostileWeb = tuitest.Hostile("pay-web")

// mixedServices are api, pay-db and the hostile pay-web in shop, in that
// order, on ports 8000-8002, each selecting one pod of its own.
func mixedServices(t *testing.T) *k8s.Client {
	t.Helper()
	names := []string{"api", "pay-db", hostileWeb}
	objs := make([]runtime.Object, 0, 1+2*len(names))
	objs = append(objs, tuitest.Namespace(shop))
	for i, n := range names {
		app := map[string]string{"app": "svc" + string(rune('a'+i))}
		objs = append(objs,
			tuitest.Service(shop, n, int32(8000+i), app),
			tuitest.Pod(shop, "pod-"+string(rune('a'+i)), app, tuitest.ContainerNginx),
		)
	}
	return tuitest.NewFakeClient(t, tuitest.WithObjects(objs...))
}

// cursorOn are the ways to put the cursor on a row whose shown index is
// not its raw one: the raw name, port and pod each lands on.
var cursorOn = []struct {
	name string
	keys []tea.Msg
	want string
	port uint16
	pod  string
}{
	{name: "a clean row first under a filter", keys: tuitest.FilterBy("pay"), want: "pay-db", port: 8001, pod: "pod-b"},
	{name: "the hostile row second under a filter", keys: on("pay", tuitest.Press("down")), want: hostileWeb, port: 8002, pod: "pod-c"},
	{name: "the hostile row alone under a filter on its shown name", keys: tuitest.FilterBy("web"), want: hostileWeb, port: 8002, pod: "pod-c"},
	{name: "the hostile row last unfiltered", keys: tuitest.Keys("down", "down"), want: hostileWeb, port: 8002, pod: "pod-c"},
}

// Every row action targets the raw name of the row under the cursor.
func TestRowActionsTargetTheRawRowUnderTheCursor(t *testing.T) {
	t.Parallel()

	for _, at := range cursorOn {
		cases := []struct {
			key   string
			check func(t *testing.T, msgs []tea.Msg)
		}{
			{key: "enter", check: func(t *testing.T, msgs []tea.Msg) {
				want := views.DescribeRequestMsg{Kind: "service", Namespace: shop, Name: at.want}
				if !tuitest.Contains(msgs, want) {
					t.Errorf("enter sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
			{key: "l", check: func(t *testing.T, msgs []tea.Msg) {
				req, ok := tuitest.Find[views.TailPromptRequestMsg](msgs)
				if !ok || req.Target.Name != at.want || req.Logs.Name != at.want || !slices.Equal(req.Pods, []string{at.pod}) {
					t.Errorf("l sent %s, want the logs of %q (pod %s)", tuitest.Describe(msgs), at.want, at.pod)
				}
			}},
			{key: "f", check: func(t *testing.T, msgs []tea.Msg) {
				want := views.ForwardRequestMsg{Kind: "service", Namespace: shop, Name: at.want, RemotePort: at.port}
				if !tuitest.Contains(msgs, want) {
					t.Errorf("f sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
		}
		for _, tc := range cases {
			t.Run(at.name+" "+tc.key, func(t *testing.T) {
				t.Parallel()
				m, _ := tuitest.Drive(t, mounted(t, mixedServices(t), shop), at.keys...)
				want := views.ResourceRef{Kind: "service", Namespace: shop, Name: at.want}
				if got, ok := m.Selected(); !ok || got != want {
					t.Fatalf("Selected() = %+v, %v, want %q", got, ok, at.want)
				}
				_, msgs := tuitest.Drive(t, m, tuitest.Press(tc.key))
				tc.check(t, msgs)
			})
		}
	}
}

// A refresh that puts a new service above the hostile row keeps the
// cursor on the same raw service, not on the index it had.
func TestRefreshKeepsTheCursorOnTheRawService(t *testing.T) {
	t.Parallel()

	c := mixedServices(t)
	m, _ := tuitest.Drive(t, mounted(t, c, shop), tuitest.Keys("down", "down")...)
	createService(t, c, tuitest.Service(shop, "aaa", 80, nil))
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got := describes(t, m); got != views.RowKey(shop, hostileWeb) {
		t.Fatalf("after the refresh enter describes %q, want the hostile service", got)
	}
	if got := m.State().Cursor; got != 3 {
		t.Errorf("cursor index = %d, want 3 (aaa, api, pay-db, then it)", got)
	}
}
