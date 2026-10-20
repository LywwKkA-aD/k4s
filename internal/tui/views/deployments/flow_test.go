package deployments_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

const (
	width  = 140
	height = 30
)

func mounted(t *testing.T, c *k8s.Client, namespace string) deployments.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, deployments.New(c, namespace), width, height)
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
			name:      "enter describes the deployment",
			namespace: demo,
			keys:      on(tuitest.DeploymentWeb, tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentWeb},
		},
		{
			name:      "enter in all namespaces reads the namespace column",
			namespace: "",
			keys:      on(tuitest.DeploymentMulti, tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentMulti},
		},
		{
			name:      "l tails every replica of a single-container template",
			namespace: demo,
			keys:      on(tuitest.DeploymentWeb, tuitest.Press("l")),
			want: views.TailPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Container: tuitest.ContainerNginx,
			},
		},
		{
			name:      "l on a multi-container template asks for the container",
			namespace: demo,
			keys:      on(tuitest.DeploymentMulti, tuitest.Press("l")),
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "logs",
			},
		},
		{
			name:      "f asks for a port-forward without a port hint",
			namespace: demo,
			keys:      on(tuitest.DeploymentWeb, tuitest.Press("f")),
			want:      views.ForwardRequestMsg{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentWeb},
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

	noPods := tuitest.NewFakeClient(t, tuitest.WithObjects(
		tuitest.Deployment(tuitest.NamespaceDemo, "idle", 0, map[string]string{"app": "idle"}, tuitest.ContainerApp),
	))
	cases := []struct {
		name string
		m    deployments.Model
		keys []tea.Msg
	}{
		{name: "e is not bound", m: mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo), keys: tuitest.Keys("e")},
		{name: "rows not loaded", m: deployments.New(tuitest.NewFakeClient(t), tuitest.NamespaceDemo), keys: tuitest.Keys("enter", "l", "f")},
		{name: "no kubeconfig", m: deployments.New(nil, ""), keys: tuitest.Keys("enter", "l", "f", "r")},
		{name: "l on a deployment without pods", m: mounted(t, noPods, tuitest.NamespaceDemo), keys: tuitest.Keys("l")},
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

	failing := tuitest.WithReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments.apps is forbidden")
	})
	cases := []struct {
		name    string
		m       func(t *testing.T) deployments.Model
		want    []string
		notWant []string
	}{
		{name: "no kubeconfig", m: func(*testing.T) deployments.Model { return deployments.New(nil, "") }, want: []string{"no kubeconfig"}},
		{
			name: "loading",
			m:    func(t *testing.T) deployments.Model { return deployments.New(tuitest.NewFakeClient(t), "") },
			want: []string{"loading deployments"},
		},
		{
			name: "table",
			m: func(t *testing.T) deployments.Model {
				return mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
			},
			want:    []string{"UP-TO-DATE", "AVAILABLE", tuitest.DeploymentWeb, "2/2"},
			notWant: []string{"NAMESPACE"},
		},
		{
			name: "all namespaces",
			m:    func(t *testing.T) deployments.Model { return mounted(t, tuitest.NewFakeClient(t), "") },
			want: []string{"NAMESPACE", tuitest.NamespaceDemo},
		},
		{
			name: "empty",
			m: func(t *testing.T) deployments.Model {
				return mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()), "")
			},
			want: []string{"no deployments"},
		},
		{
			name: "filter without a match",
			m: func(t *testing.T) deployments.Model {
				m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), ""), tuitest.FilterBy("zzz")...)
				return m
			},
			want: []string{"no deployments match /zzz"},
		},
		{
			name: "fetch error",
			m:    func(t *testing.T) deployments.Model { return mounted(t, tuitest.NewFakeClient(t, failing), "") },
			want: []string{"deployments unavailable: deployments.apps is forbidden"},
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
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("mul"))
	if !m.CapturesKeys() {
		t.Fatal("open filter must capture keys")
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, tuitest.DeploymentMulti) || strings.Contains(screen, " web ") {
		t.Errorf("filter narrows rows:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if m.CapturesKeys() || strings.Contains(tuitest.Screen(m), "/ mul") {
		t.Errorf("esc closes and clears the filter:\n%s", tuitest.Screen(m))
	}
}

func TestWatchToggle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace string
		watching  string
		paused    string
	}{
		{namespace: "", watching: "kubectl get deployments -A --watch", paused: "kubectl get deployments -A"},
		{namespace: tuitest.NamespaceDemo, watching: "kubectl get deployments -n demo --watch", paused: "kubectl get deployments -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.watching, func(t *testing.T) {
			t.Parallel()
			m := deployments.New(nil, tc.namespace)
			if got := m.KubectlEquivalent(); got != tc.watching {
				t.Errorf("default = %q, want %q", got, tc.watching)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
			if got := m.KubectlEquivalent(); got != tc.paused {
				t.Errorf("after w = %q, want %q", got, tc.paused)
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

	err := c.Clientset.AppsV1().Deployments(tuitest.NamespaceDemo).Delete(context.Background(), tuitest.DeploymentMulti, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("delete deployment: %v", err)
	}
	m, _ = tuitest.Drive(t, m, first...)
	m, third := tuitest.Step(t, m, tuitest.Press("r"))
	if len(third) != 1 {
		t.Fatalf("the reply releases busy:\n%s", tuitest.Describe(third))
	}
	m, _ = tuitest.Drive(t, m, third...)
	if strings.Contains(tuitest.Screen(m), tuitest.DeploymentMulti) {
		t.Errorf("refresh did not drop the deleted deployment:\n%s", tuitest.Screen(m))
	}
}

func TestStaleGenerationReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, deployments.New(c, tuitest.NamespaceDemo))
	fresh, _ := tuitest.Drive(t, deployments.New(c, tuitest.NamespaceDemo), tuitest.Resize(width, height))
	fresh, inFlight := tuitest.Step(t, fresh, tuitest.Press("r"))
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if !strings.Contains(tuitest.Screen(fresh), "loading deployments") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
	if _, again := tuitest.Step(t, fresh, tuitest.Press("r")); len(again) != 0 {
		t.Errorf("the stale reply released the busy flag:\n%s", tuitest.Describe(again))
	}
	fresh, _ = tuitest.Drive(t, fresh, inFlight...)
	if !strings.Contains(tuitest.Screen(fresh), tuitest.DeploymentWeb) {
		t.Errorf("own reply not applied:\n%s", tuitest.Screen(fresh))
	}
}

func TestStaticContract(t *testing.T) {
	t.Parallel()

	m := deployments.New(nil, "")
	if m.Title() != "deployments" || m.CapturesKeys() || m.Close() != nil {
		t.Errorf("Title=%q CapturesKeys=%v", m.Title(), m.CapturesKeys())
	}
	names := tuitest.HelpKeys(m)
	if want := []string{"enter", "l", "f", "/", "w", "r"}; !slices.Equal(names, want) {
		t.Errorf("help keys = %v, want %v", names, want)
	}
}
