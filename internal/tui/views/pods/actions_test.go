package pods_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const demo = tuitest.NamespaceDemo

// In demo the rows are crash-0, multi-0, web-1, web-2.
const (
	rowMulti = 1
	rowWeb1  = 2
	rowWeb2  = 3
)

// TestLogsLastWins: l on another row replaces the resolve in flight, whose
// reply is then dropped (M3.8 scenario 1).
func TestLogsLastWins(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), demo)
	m, _ = tuitest.Drive(t, m, down(rowWeb1)...)
	m, first := tuitest.Step(t, m, tuitest.Press("l"))
	m, _ = tuitest.Drive(t, m, down(1)...)
	m, second := tuitest.Step(t, m, tuitest.Press("l"))

	m, out := tuitest.Drive(t, m, first...)
	if reqs := tuitest.Requests(out); len(reqs) != 0 {
		t.Fatalf("the superseded resolve still answered:\n%s", tuitest.Describe(reqs))
	}
	_, out = tuitest.Drive(t, m, second...)
	want := views.TailPromptRequestMsg{
		Namespace: demo, Pods: []string{tuitest.PodWeb2}, Container: tuitest.ContainerNginx,
		Target: podRef(demo, tuitest.PodWeb2),
	}
	if reqs := tuitest.Requests(out); len(reqs) != 1 || !tuitest.Contains(reqs, want) {
		t.Fatalf("want only the prompt for web-2, got:\n%s", tuitest.Describe(reqs))
	}
}

// TestRepeatedLogsOnTheSameRowResolvesOnce: a second l on the row already
// resolving starts nothing.
func TestRepeatedLogsOnTheSameRowResolvesOnce(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), demo)
	m, first := tuitest.Step(t, m, tuitest.Press("l"))
	m, again := tuitest.Step(t, m, tuitest.Press("l"))
	if len(again) != 0 {
		t.Fatalf("a repeated l resolved again:\n%s", tuitest.Describe(again))
	}
	m, out := tuitest.Drive(t, m, first...)
	if n := len(tuitest.All[views.TailPromptRequestMsg](out)); n != 1 {
		t.Fatalf("want one prompt, got %d:\n%s", n, tuitest.Describe(out))
	}
	// Once answered, l resolves again.
	if _, out = tuitest.Step(t, m, tuitest.Press("l")); len(out) != 1 {
		t.Fatalf("l after the answer: %s", tuitest.Describe(out))
	}
}

// TestDetachedWhenTheCursorMoved: the reply still opens the prompt for the
// row it was asked for, marked as not the selected row (M3.8 scenario 2).
func TestDetachedWhenTheCursorMoved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		row  int
		key  string
		want tea.Msg
	}{
		{
			name: "logs of a single-container pod", row: rowWeb1, key: "l",
			want: views.TailPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodWeb1}, Container: tuitest.ContainerNginx,
				Target: podRef(demo, tuitest.PodWeb1), Detached: true,
			},
		},
		{
			name: "logs of a multi-container pod", row: rowMulti, key: "l",
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "logs",
				Target: podRef(demo, tuitest.PodMulti), Detached: true,
			},
		},
		{
			name: "exec in a multi-container pod", row: rowMulti, key: "e",
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "exec",
				Target: podRef(demo, tuitest.PodMulti), Detached: true, Shell: views.ExecShell,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := mounted(t, tuitest.NewFakeClient(t), demo)
			m, _ = tuitest.Drive(t, m, down(tc.row)...)
			m, reply := tuitest.Step(t, m, tuitest.Press(tc.key))
			m, _ = tuitest.Drive(t, m, tuitest.Press("up"))
			_, out := tuitest.Drive(t, m, reply...)
			if reqs := tuitest.Requests(out); len(reqs) != 1 || !tuitest.Contains(reqs, tc.want) {
				t.Fatalf("want %+v, got:\n%s", tc.want, tuitest.Describe(reqs))
			}
		})
	}
}

// initPod is a pod with one app container after a plain init container,
// and a native sidecar before it when sidecar is set.
func initPod(name string, sidecar bool) *corev1.Pod {
	p := tuitest.Pod(demo, name, nil, "app")
	p.Spec.InitContainers = []corev1.Container{{Name: "migrate"}}
	if sidecar {
		always := corev1.ContainerRestartPolicyAlways
		p.Spec.InitContainers = []corev1.Container{{Name: "log-shipper", RestartPolicy: &always}, {Name: "migrate"}}
	}
	return p
}

// TestLookupOffersContainersByAction: l offers every container, the init
// containers labelled, since a pod stuck in Init says why in their logs;
// e offers the containers a shell runs in, the app containers and native
// sidecars, and goes straight in when there is one.
func TestLookupOffersContainersByAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pod  *corev1.Pod
		key  string
		want tea.Msg
	}{
		{
			name: "logs of a pod with init containers", pod: initPod("init-0", true), key: "l",
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{"init-0"}, NextKind: "logs", Target: podRef(demo, "init-0"),
				Containers: []string{"app", "log-shipper", "migrate"}, Init: []string{"log-shipper", "migrate"},
			},
		},
		{
			name: "exec offers the app container and the sidecar", pod: initPod("init-0", true), key: "e",
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{"init-0"}, NextKind: "exec", Target: podRef(demo, "init-0"),
				Containers: []string{"app", "log-shipper"}, Init: []string{"log-shipper"}, Shell: views.ExecShell,
			},
		},
		{
			name: "logs of one app container behind an init container ask which", pod: initPod("migrating-0", false), key: "l",
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{"migrating-0"}, NextKind: "logs", Target: podRef(demo, "migrating-0"),
				Containers: []string{"app", "migrate"}, Init: []string{"migrate"},
			},
		},
		{
			name: "exec goes straight into the one app container", pod: initPod("migrating-0", false), key: "e",
			want: views.ExecRequestMsg{Namespace: demo, Pod: "migrating-0", Container: "app", Shell: views.ExecShell},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(demo), tc.pod))
			_, out := tuitest.Drive(t, mounted(t, c, demo), tuitest.Press(tc.key))
			if reqs := tuitest.Requests(out); len(reqs) != 1 || !tuitest.Contains(reqs, tc.want) {
				t.Fatalf("want %+v, got:\n%s", tc.want, tuitest.Describe(reqs))
			}
		})
	}
}

// TestLookupRepliesWithTheContainers: the lookup's reply is the typed
// containers the action offers, so nothing reads an init flag out of a
// name.
func TestLookupRepliesWithTheContainers(t *testing.T) {
	t.Parallel()

	app := k8s.Container{Name: "app"}
	shipper := k8s.Container{Name: "log-shipper", Init: true, Sidecar: true}
	migrate := k8s.Container{Name: "migrate", Init: true}
	cases := []struct {
		key  string
		want k8s.Containers
	}{
		{key: "l", want: k8s.Containers{app, shipper, migrate}},
		{key: "e", want: k8s.Containers{app, shipper}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(demo), initPod("init-0", true)))
			_, out := tuitest.Drive(t, mounted(t, c, demo), tuitest.Press(tc.key))
			reply, ok := tuitest.Find[async.Result[k8s.Containers]](out)
			if !ok {
				t.Fatalf("no typed lookup reply:\n%s", tuitest.Describe(out))
			}
			if reply.Err != nil || !slices.Equal(reply.Value, tc.want) {
				t.Errorf("reply = %+v, %v; want %+v", reply.Value, reply.Err, tc.want)
			}
		})
	}
}

// windowsPod runs on Windows, as spec.os says or, for an older pod, as its
// node selector pins the node OS.
func windowsPod(name string, byNodeSelector bool, containers ...string) *corev1.Pod {
	p := tuitest.Pod(demo, name, nil, containers...)
	if byNodeSelector {
		p.Spec.NodeSelector = map[string]string{corev1.LabelOSStable: "windows"}
		return p
	}
	p.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
	return p
}

// TestExecNamesTheShell: e asks for cmd.exe in a Windows pod and sh in any
// other, so the picker's preview, the footer and what runs agree.
func TestExecNamesTheShell(t *testing.T) {
	t.Parallel()

	linux := tuitest.Pod(demo, "web-0", nil, "app")
	linux.Spec.OS = &corev1.PodOS{Name: corev1.Linux}
	cases := []struct {
		name string
		pod  *corev1.Pod
		want tea.Msg
	}{
		{
			name: "windows by spec.os", pod: windowsPod("iis-0", false, "web"),
			want: views.ExecRequestMsg{Namespace: demo, Pod: "iis-0", Container: "web", Shell: views.WindowsShell},
		},
		{
			name: "windows by the node selector", pod: windowsPod("iis-1", true, "web"),
			want: views.ExecRequestMsg{Namespace: demo, Pod: "iis-1", Container: "web", Shell: views.WindowsShell},
		},
		{
			name: "a windows pod with two containers", pod: windowsPod("iis-2", false, "web", "agent"),
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{"iis-2"}, Containers: []string{"web", "agent"}, NextKind: "exec",
				Target: podRef(demo, "iis-2"), Shell: views.WindowsShell,
			},
		},
		{
			name: "a linux pod", pod: linux,
			want: views.ExecRequestMsg{Namespace: demo, Pod: "web-0", Container: "app", Shell: views.ExecShell},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(demo), tc.pod))
			_, out := tuitest.Drive(t, mounted(t, c, demo), tuitest.Press("e"))
			if reqs := tuitest.Requests(out); len(reqs) != 1 || !tuitest.Contains(reqs, tc.want) {
				t.Fatalf("want %+v, got:\n%s", tc.want, tuitest.Describe(reqs))
			}
		})
	}
}

// TestExecAndForwardFirstWins: a second e or f while the first resolves
// emits nothing, so one key press opens one shell or one prompt.
func TestExecAndForwardFirstWins(t *testing.T) {
	t.Parallel()

	cases := []struct {
		first, second string
	}{
		{first: "e", second: "e"},
		{first: "f", second: "f"},
		{first: "l", second: "f"},
		{first: "e", second: "f"},
	}
	for _, tc := range cases {
		t.Run(tc.first+" then "+tc.second, func(t *testing.T) {
			t.Parallel()
			m := mounted(t, tuitest.NewFakeClient(t), demo)
			m, _ = tuitest.Drive(t, m, down(rowWeb1)...)
			m, reply := tuitest.Step(t, m, tuitest.Press(tc.first))
			m, again := tuitest.Step(t, m, tuitest.Press(tc.second))
			if len(again) != 0 {
				t.Fatalf("%s while %s resolves emitted:\n%s", tc.second, tc.first, tuitest.Describe(again))
			}
			_, out := tuitest.Drive(t, m, reply...)
			if reqs := tuitest.Requests(out); len(reqs) != 1 {
				t.Fatalf("want one request, got:\n%s", tuitest.Describe(reqs))
			}
		})
	}
}

// TestResolvingNoticeAfter150ms: a resolve still in flight after 150 ms
// says so; one answered in time says nothing.
func TestResolvingNoticeAfter150ms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want string
	}{
		{key: "l", want: "resolving logs for pod demo/web-1…"},
		{key: "e", want: "resolving exec for pod demo/web-1…"},
		{key: "f", want: "resolving port-forward for pod demo/web-1…"},
	}
	for _, tc := range cases {
		t.Run(tc.key+" still resolving", func(t *testing.T) {
			t.Parallel()
			clk := asynctest.NewClock(epoch)
			m := mountedAt(t, tuitest.NewFakeClient(t), demo, clk)
			m, _ = tuitest.Drive(t, m, down(rowWeb1)...)
			m, _ = tuitest.Step(t, m, tuitest.Press(tc.key))

			if early := clk.Advance(149 * time.Millisecond); len(early) != 0 {
				t.Fatalf("notice before 150ms: %v", early)
			}
			_, out := tuitest.Drive(t, m, clk.Advance(time.Millisecond)...)
			want := views.StatusMsg{Level: status.Info, Text: tc.want}
			if len(out) != 1 || out[0] != want {
				t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
			}
		})
		t.Run(tc.key+" answered in time", func(t *testing.T) {
			t.Parallel()
			clk := asynctest.NewClock(epoch)
			m := mountedAt(t, tuitest.NewFakeClient(t), demo, clk)
			m, _ = tuitest.Drive(t, m, down(rowWeb1)...)
			m, reply := tuitest.Step(t, m, tuitest.Press(tc.key))
			m, _ = tuitest.Drive(t, m, reply...)
			if _, out := tuitest.Drive(t, m, clk.Advance(150*time.Millisecond)...); len(out) != 0 {
				t.Fatalf("notice after the answer:\n%s", tuitest.Describe(out))
			}
		})
	}
}

// TestRowActionFailuresSaySo: every way a row action can fail ends in the
// status line (M3.2).
func TestRowActionFailuresSaySo(t *testing.T) {
	t.Parallel()

	forbidden := tuitest.WithReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		denial := errors.New(`User "u" cannot get resource "pods" in API group "" in the namespace "demo"`)
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "web-1", denial)
	})
	hollow := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "hollow", Namespace: demo, CreationTimestamp: metav1.Now()}}
	actions := map[string]string{"l": "logs", "e": "exec", "f": "port-forward"}
	cases := []struct {
		name string
		opts []tuitest.FakeOption
		row  int
		gone bool
		keys []string
		want func(key string) views.StatusMsg
	}{
		{
			name: "pod deleted since the list", row: rowWeb1, gone: true, keys: []string{"l", "e", "f"},
			want: func(string) views.StatusMsg {
				return views.StatusMsg{Level: status.Warn, Text: "pod web-1 gone, press r"}
			},
		},
		{
			name: "lookup error", opts: []tuitest.FakeOption{forbidden}, row: rowWeb1, keys: []string{"l", "e", "f"},
			want: func(key string) views.StatusMsg {
				return views.StatusMsg{Level: status.Error, Text: actions[key] + ` for pod demo/web-1: forbidden: get pods in demo · kubectl auth can-i get pods -n demo`}
			},
		},
		{
			name: "pod without containers", opts: []tuitest.FakeOption{tuitest.WithObjects(tuitest.Namespace(demo), hollow)}, keys: []string{"l", "e"},
			want: func(string) views.StatusMsg {
				return views.StatusMsg{Level: status.Warn, Text: "pod hollow has no containers"}
			},
		},
	}
	for _, tc := range cases {
		for _, k := range tc.keys {
			t.Run(tc.name+" "+k, func(t *testing.T) {
				t.Parallel()
				c := tuitest.NewFakeClient(t, tc.opts...)
				m, _ := tuitest.Drive(t, mounted(t, c, demo), down(tc.row)...)
				if tc.gone {
					if err := c.Clientset.CoreV1().Pods(demo).Delete(context.Background(), tuitest.PodWeb1, metav1.DeleteOptions{}); err != nil {
						t.Fatalf("delete: %v", err)
					}
				}
				_, out := tuitest.Drive(t, m, tuitest.Press(k))
				out = tuitest.Requests(out)
				want := tc.want(k)
				if len(out) != 1 || out[0] != want {
					t.Fatalf("want only %+v, got:\n%s", want, tuitest.Describe(out))
				}
			})
		}
	}
}

// TestActionReplyAfterTheViewMovedOnIsDropped: a reply that lands in a
// rebuilt view (another instance) opens nothing.
func TestActionReplyAfterTheViewMovedOnIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	m := mounted(t, c, demo)
	_, reply := tuitest.Step(t, m, tuitest.Press("l"))
	fresh := mounted(t, c, demo)
	if _, out := tuitest.Drive(t, fresh, reply...); len(out) != 0 {
		t.Fatalf("another view's resolve answered here:\n%s", tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(fresh), tuitest.PodWeb1) {
		t.Fatal("the fresh view lost its rows")
	}
}
