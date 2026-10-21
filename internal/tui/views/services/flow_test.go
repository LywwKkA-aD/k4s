package services_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

func TestRowActionsEmitRequests(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name      string
		namespace string
		keys      []tea.Msg
		want      tea.Msg
	}{
		{
			name:      "enter describes the service",
			namespace: demo,
			keys:      on(tuitest.ServiceWeb, tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "service", Namespace: demo, Name: tuitest.ServiceWeb},
		},
		{
			name:      "enter in all namespaces reads the namespace column",
			namespace: "",
			keys:      on(tuitest.ServiceMulti, tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "service", Namespace: demo, Name: tuitest.ServiceMulti},
		},
		{
			name:      "f suggests the first service port",
			namespace: demo,
			keys:      on(tuitest.ServiceWeb, tuitest.Press("f")),
			want:      views.ForwardRequestMsg{Kind: "service", Namespace: demo, Name: tuitest.ServiceWeb, RemotePort: tuitest.ServiceWebPort},
		},
		{
			name:      "f in all namespaces finds the port too",
			namespace: "",
			keys:      on(tuitest.ServiceMulti, tuitest.Press("f")),
			want:      views.ForwardRequestMsg{Kind: "service", Namespace: demo, Name: tuitest.ServiceMulti, RemotePort: tuitest.ServiceMultiPort},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), tc.namespace), tc.keys...)
			if !tuitest.Contains(out, tc.want) {
				t.Errorf("want %T %+v among:\n%s", tc.want, tc.want, tuitest.Describe(out))
			}
		})
	}
}

// TestKeysThatCannotActSaySo: a row key without a row to act on answers
// with a status, never silently; e is not a services key at all.
func TestKeysThatCannotActSaySo(t *testing.T) {
	t.Parallel()

	loading, _ := clocked(tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	cases := []struct {
		name string
		m    services.Model
		keys []tea.Msg
		want []views.StatusMsg
	}{
		{name: "e is not bound", m: mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo), keys: tuitest.Keys("e")},
		{
			name: "rows not loaded", m: loading, keys: tuitest.Keys("enter", "l", "f"),
			want: repeatStatus(3, status.Info, "services are still loading"),
		},
		{
			name: "no kubeconfig", m: services.New(nil, ""), keys: tuitest.Keys("enter", "l", "f", "r", "w"),
			want: repeatStatus(5, status.Warn, "no kubeconfig"),
		},
		{
			name: "no row under the cursor", m: mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo),
			keys: on("zzz", tuitest.Keys("enter", "l", "f")...),
			want: repeatStatus(3, status.Warn, "no service selected"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, tc.m, tc.keys...)
			if got := tuitest.Requests(out); !slices.Equal(statuses(got), tc.want) || len(got) != len(tc.want) {
				t.Errorf("emitted:\n%s\nwant %+v", tuitest.Describe(got), tc.want)
			}
		})
	}
}

func repeatStatus(n int, level status.Level, text string) []views.StatusMsg {
	out := make([]views.StatusMsg, 0, n)
	for range n {
		out = append(out, views.StatusMsg{Level: level, Text: text})
	}
	return out
}

func TestViewStates(t *testing.T) {
	t.Parallel()

	failing := tuitest.WithReactor("list", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("services is forbidden")
	})
	cases := []struct {
		name    string
		m       func(t *testing.T) services.Model
		want    []string
		notWant []string
	}{
		{name: "no kubeconfig", m: func(*testing.T) services.Model { return services.New(nil, "") }, want: []string{"no kubeconfig"}},
		{
			name: "loading",
			m:    func(t *testing.T) services.Model { return services.New(tuitest.NewFakeClient(t), "") },
			want: []string{"loading services"},
		},
		{
			name:    "table",
			m:       func(t *testing.T) services.Model { return mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo) },
			want:    []string{"TYPE", "CLUSTER-IP", "PORT(S)", "ClusterIP", "80/TCP", "8080/TCP"},
			notWant: []string{"NAMESPACE"},
		},
		{
			name: "all namespaces",
			m:    func(t *testing.T) services.Model { return mounted(t, tuitest.NewFakeClient(t), "") },
			want: []string{"NAMESPACE", tuitest.NamespaceDemo},
		},
		{
			name: "empty",
			m: func(t *testing.T) services.Model {
				return mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()), "")
			},
			want: []string{"no services"},
		},
		{
			name: "filter without a match",
			m: func(t *testing.T) services.Model {
				m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), ""), tuitest.FilterBy("zzz")...)
				return m
			},
			want: []string{"no services match /zzz"},
		},
		{
			name: "fetch error",
			m:    func(t *testing.T) services.Model { return mounted(t, tuitest.NewFakeClient(t, failing), "") },
			want: []string{"services unavailable: list services: services is forbidden"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			screen := tuitest.Screen(tc.m(t))
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks %q:\n%s", w, screen)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(screen, w) {
					t.Errorf("screen shows %q:\n%s", w, screen)
				}
			}
		})
	}
}

func TestFilterCapturesKeys(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("w"))
	if !m.CapturesKeys() {
		t.Fatal("open filter must capture keys")
	}
	// w is filter text here, not the watch toggle.
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ w") {
		t.Errorf("w inside the filter did not reach it:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if m.CapturesKeys() {
		t.Error("esc closes the filter")
	}
}

// TestWatchToggleKeepsTheCommand: k4s polls, kubectl --watch streams, so
// the footer command is the same with auto-refresh on or off.
func TestWatchToggleKeepsTheCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace string
		want      string
	}{
		{namespace: "", want: "kubectl get services -A"},
		{namespace: tuitest.NamespaceDemo, want: "kubectl get services -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			m := services.New(nil, tc.namespace)
			if got := m.Command().String(); got != tc.want {
				t.Errorf("default = %q, want %q", got, tc.want)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
			if got := m.Command().String(); got != tc.want {
				t.Errorf("after w = %q, want %q", got, tc.want)
			}
		})
	}
}

// onlyRefreshNotice reports that msgs are the one status r gives while a
// list is in flight, with no fetch behind it.
func onlyRefreshNotice(msgs []tea.Msg) bool {
	return len(msgs) == 1 && msgs[0] == views.StatusMsg{Level: status.Info, Text: "refresh already running"}
}

func TestRefreshRespectsBusy(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	m := mounted(t, c, tuitest.NamespaceDemo)
	m, first := tuitest.Step(t, m, tuitest.Press("r"))
	if len(first) != 1 {
		t.Fatalf("r should issue one fetch:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("r"))
	if !onlyRefreshNotice(second) {
		t.Fatalf("r while busy must only say so, not fetch again:\n%s", tuitest.Describe(second))
	}
	deleteService(t, c, tuitest.NamespaceDemo, tuitest.ServiceMulti)
	m, _ = tuitest.Drive(t, m, first...)
	m, third := tuitest.Step(t, m, tuitest.Press("r"))
	if len(third) != 1 {
		t.Fatalf("the reply releases busy:\n%s", tuitest.Describe(third))
	}
	m, _ = tuitest.Drive(t, m, third...)
	if strings.Contains(tuitest.Screen(m), "8080/TCP") {
		t.Errorf("refresh did not drop the deleted service:\n%s", tuitest.Screen(m))
	}
}

// TestAnotherViewsReplyIsDropped: replies carry the token of the view
// that asked, so a rebuilt view ignores its predecessor's.
func TestAnotherViewsReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, services.New(c, tuitest.NamespaceDemo))
	fresh, _ := tuitest.Drive(t, services.New(c, tuitest.NamespaceDemo), tuitest.Resize(width, height))
	inFlight := tuitest.Collect(t, fresh.Init())
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if !strings.Contains(tuitest.Screen(fresh), "loading services") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
	if _, again := tuitest.Step(t, fresh, tuitest.Press("r")); !onlyRefreshNotice(again) {
		t.Errorf("the stale reply released the busy flag:\n%s", tuitest.Describe(again))
	}
	fresh, _ = tuitest.Drive(t, fresh, inFlight...)
	if !strings.Contains(tuitest.Screen(fresh), "80/TCP") {
		t.Errorf("own reply not applied:\n%s", tuitest.Screen(fresh))
	}
}

func TestStaticContract(t *testing.T) {
	t.Parallel()

	m := services.New(nil, "")
	if m.Title() != "services" || m.CapturesKeys() || m.Close() != nil {
		t.Errorf("Title=%q CapturesKeys=%v", m.Title(), m.CapturesKeys())
	}
	names := tuitest.HelpKeys(m)
	if want := []string{"enter", "l", "f", "/", "w", "r"}; !slices.Equal(names, want) {
		t.Errorf("help keys = %v, want %v", names, want)
	}
}
