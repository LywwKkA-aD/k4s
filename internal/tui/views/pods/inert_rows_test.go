package pods_test

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostileWeb is the raw name of the one hostile pod among clean ones. It
// sorts after them (U+202E is past ASCII), so it is never raw index 0.
var hostileWeb = tuitest.Hostile("pay-web")

// mixedPods are api, pay-db and the hostile pay-web in shop, in that
// order, each with one nginx container.
func mixedPods(t *testing.T) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, tuitest.WithObjects(
		tuitest.Namespace(shop),
		tuitest.Pod(shop, "api", nil, tuitest.ContainerNginx),
		tuitest.Pod(shop, "pay-db", nil, tuitest.ContainerNginx),
		tuitest.Pod(shop, hostileWeb, nil, tuitest.ContainerNginx),
	))
}

// cursorOn are the ways to put the cursor on a row whose shown index is
// not its raw one, and the raw name each lands on.
var cursorOn = []struct {
	name string
	keys []tea.Msg
	want string
}{
	{name: "a clean row first under a filter", keys: tuitest.FilterBy("pay"), want: "pay-db"},
	{name: "the hostile row second under a filter", keys: keys(tuitest.FilterBy("pay"), tuitest.Press("down")), want: hostileWeb},
	{name: "the hostile row alone under a filter on its shown name", keys: tuitest.FilterBy("web"), want: hostileWeb},
	{name: "the hostile row last unfiltered", keys: down(2), want: hostileWeb},
}

// Every row action targets the raw name of the row under the cursor, and
// a command built from the hostile one is none at all.
func TestRowActionsTargetTheRawRowUnderTheCursor(t *testing.T) {
	t.Parallel()

	for _, at := range cursorOn {
		hostile := at.want == hostileWeb
		cases := []struct {
			key   string
			check func(t *testing.T, msgs []tea.Msg)
		}{
			{key: "enter", check: func(t *testing.T, msgs []tea.Msg) {
				want := views.DescribeRequestMsg{Kind: "pod", Namespace: shop, Name: at.want}
				if !tuitest.Contains(msgs, want) {
					t.Errorf("enter sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
			{key: "l", check: func(t *testing.T, msgs []tea.Msg) {
				req, ok := tuitest.Find[views.TailPromptRequestMsg](msgs)
				if !ok || !slices.Equal(req.Pods, []string{at.want}) || req.Target.Name != at.want {
					t.Fatalf("l sent %s, want the tail prompt for %q", tuitest.Describe(msgs), at.want)
				}
				cmd := views.LogsCommand(req.Namespace, req.Pods, req.Container, 100, req.Logs)
				if cmd.IsZero() != hostile {
					t.Errorf("logs command %q for %q, want none only for the hostile row", cmd.Display(), at.want)
				}
			}},
			{key: "e", check: func(t *testing.T, msgs []tea.Msg) {
				req, ok := tuitest.Find[views.ExecRequestMsg](msgs)
				if !ok || req.Pod != at.want || req.Namespace != shop {
					t.Fatalf("e sent %s, want an exec in %q", tuitest.Describe(msgs), at.want)
				}
				if _, err := views.ExecCommand(req); (err != nil) != hostile {
					t.Errorf("exec command for %q: err %v, want one only for the hostile row", at.want, err)
				}
			}},
			{key: "f", check: func(t *testing.T, msgs []tea.Msg) {
				want := views.ForwardRequestMsg{Kind: "pod", Namespace: shop, Name: at.want}
				if !tuitest.Contains(msgs, want) {
					t.Errorf("f sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
		}
		for _, tc := range cases {
			t.Run(at.name+" "+tc.key, func(t *testing.T) {
				t.Parallel()
				m, _ := tuitest.Drive(t, mounted(t, mixedPods(t), shop), at.keys...)
				if got, ok := m.Selected(); !ok || got != podRef(shop, at.want) {
					t.Fatalf("Selected() = %+v, %v, want %q", got, ok, at.want)
				}
				_, msgs := tuitest.Drive(t, m, tuitest.Press(tc.key))
				tc.check(t, msgs)
			})
		}
	}
}

// A refresh that puts a new pod above the hostile row keeps the cursor
// on the same raw pod, not on the index it had.
func TestRefreshKeepsTheCursorOnTheRawPod(t *testing.T) {
	t.Parallel()

	c := mixedPods(t)
	m, _ := tuitest.Drive(t, mounted(t, c, shop), down(2)...)
	if _, err := c.Clientset.CoreV1().Pods(shop).Create(context.Background(),
		tuitest.Pod(shop, "aaa", nil, tuitest.ContainerNginx), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got, ok := m.Selected(); !ok || got != podRef(shop, hostileWeb) {
		t.Fatalf("after the refresh Selected() = %+v, %v, want the hostile pod", got, ok)
	}
	if got := m.State().Cursor; got != 3 {
		t.Errorf("cursor index = %d, want 3 (aaa, api, pay-db, then it)", got)
	}
}
