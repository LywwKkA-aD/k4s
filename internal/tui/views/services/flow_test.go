package services_test

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
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

const (
	width  = 160
	height = 30
)

func mounted(t *testing.T, c *k8s.Client, namespace string) services.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, services.New(c, namespace), width, height)
	return m
}

func on(name string, more ...tea.Msg) []tea.Msg {
	return slices.Concat(tuitest.FilterBy(name), more)
}

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
			name:      "l tails every pod behind a single-container service",
			namespace: demo,
			keys:      on(tuitest.ServiceWeb, tuitest.Press("l")),
			want: views.TailPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Container: tuitest.ContainerNginx,
			},
		},
		{
			name:      "l behind multi-container pods asks for the container",
			namespace: demo,
			keys:      on(tuitest.ServiceMulti, tuitest.Press("l")),
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "logs",
			},
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

func TestKeysWithoutAnAction(t *testing.T) {
	t.Parallel()

	headless := tuitest.Service(tuitest.NamespaceDemo, "external", 443, nil)
	selectorless := tuitest.NewFakeClient(t, tuitest.WithObjects(headless))
	cases := []struct {
		name string
		m    services.Model
		keys []tea.Msg
	}{
		{name: "e is not bound", m: mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo), keys: tuitest.Keys("e")},
		{name: "rows not loaded", m: services.New(tuitest.NewFakeClient(t), tuitest.NamespaceDemo), keys: tuitest.Keys("enter", "l", "f")},
		{name: "no kubeconfig", m: services.New(nil, ""), keys: tuitest.Keys("enter", "l", "f", "r")},
		{name: "l on a service without a selector", m: mounted(t, selectorless, tuitest.NamespaceDemo), keys: tuitest.Keys("l")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, out := tuitest.Drive(t, tc.m, tc.keys...); len(tuitest.Requests(out)) != 0 {
				t.Errorf("emitted:\n%s", tuitest.Describe(out))
			}
		})
	}
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

func TestRefreshRespectsBusy(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	m := mounted(t, c, tuitest.NamespaceDemo)
	m, first := tuitest.Step(t, m, tuitest.Press("r"))
	if len(first) != 1 {
		t.Fatalf("r should issue one fetch:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("r"))
	if len(second) != 0 {
		t.Fatalf("r while busy must not fetch again:\n%s", tuitest.Describe(second))
	}
	if err := c.Clientset.CoreV1().Services(tuitest.NamespaceDemo).Delete(context.Background(), tuitest.ServiceMulti, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete service: %v", err)
	}
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

func TestStaleGenerationReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, services.New(c, tuitest.NamespaceDemo))
	fresh, _ := tuitest.Drive(t, services.New(c, tuitest.NamespaceDemo), tuitest.Resize(width, height))
	fresh, inFlight := tuitest.Step(t, fresh, tuitest.Press("r"))
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if !strings.Contains(tuitest.Screen(fresh), "loading services") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
	if _, again := tuitest.Step(t, fresh, tuitest.Press("r")); len(again) != 0 {
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
