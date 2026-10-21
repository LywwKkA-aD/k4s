package deployments_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

func TestRowActionsEmitRequests(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	api, worker := target(t, c, "api"), target(t, c, "worker")
	cases := []struct {
		name      string
		namespace string
		keys      []tea.Msg
		want      tea.Msg
	}{
		{
			name:      "enter describes the deployment",
			namespace: shop,
			keys:      on("api", tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: "api"},
		},
		{
			name:      "enter in all namespaces reads the namespace column",
			namespace: "",
			keys:      on("worker", tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: "worker"},
		},
		{
			name:      "l tails the running pods of a single-container template",
			namespace: shop,
			keys:      on("api", tuitest.Press("l")),
			want: views.TailPromptRequestMsg{
				Namespace: shop, Pods: []string{"api-h1-a", "api-h1-b"}, Container: appContainer,
				Target: ref("api"), Logs: api,
			},
		},
		{
			name:      "l on a multi-container template asks for the container",
			namespace: "",
			keys:      on("worker", tuitest.Press("l")),
			want: views.ContainerPromptRequestMsg{
				Namespace: shop, Pods: []string{"worker-h1-a"}, Containers: []string{appContainer, sidecarContainer},
				NextKind: "logs", Target: ref("worker"), Logs: worker,
			},
		},
		{
			name:      "f asks for a port-forward without a port hint",
			namespace: shop,
			keys:      on("api", tuitest.Press("f")),
			want:      views.ForwardRequestMsg{Kind: "deployment", Namespace: shop, Name: "api"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, mounted(t, c, tc.namespace), tc.keys...)
			if !tuitest.Contains(out, tc.want) {
				t.Errorf("want %T %+v among:\n%s", tc.want, tc.want, tuitest.Describe(out))
			}
			if got := statuses(out); len(got) != 0 {
				t.Errorf("a working action set a status: %+v", got)
			}
		})
	}
}

// TestKeysThatCannotActSayWhy: a row key that cannot do its job answers
// with a status, never with silence.
func TestKeysThatCannotActSayWhy(t *testing.T) {
	t.Parallel()

	shopClient := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	failing := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...),
		tuitest.WithReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("etcdserver: request timed out")
		}))
	unlisted := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...),
		tuitest.WithReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("deployments.apps is forbidden")
		}))
	starting := tuitest.NewFakeClient(t, tuitest.WithObjects(
		tuitest.Deployment(shop, "starting", 1, map[string]string{"app": "starting"}, appContainer)))
	cases := []struct {
		name string
		m    func(t *testing.T) deployments.Model
		keys []tea.Msg
		want views.StatusMsg
	}{
		{
			name: "no kubeconfig", m: func(*testing.T) deployments.Model { return deployments.New(nil, "") },
			keys: tuitest.Keys("l"), want: views.StatusMsg{Level: status.Warn, Text: "no kubeconfig"},
		},
		{
			name: "r without a kubeconfig", m: func(*testing.T) deployments.Model { return deployments.New(nil, "") },
			keys: tuitest.Keys("r"), want: views.StatusMsg{Level: status.Warn, Text: "no kubeconfig"},
		},
		{
			name: "rows not loaded", m: func(t *testing.T) deployments.Model { return deployments.New(shopClient, shop) },
			keys: tuitest.Keys("enter"), want: views.StatusMsg{Level: status.Info, Text: "deployments are still loading"},
		},
		{
			name: "list failed", m: func(t *testing.T) deployments.Model { return mounted(t, unlisted, shop) },
			keys: tuitest.Keys("l"), want: views.StatusMsg{Level: status.Warn, Text: "deployments unavailable, press r"},
		},
		{
			name: "l before any pod runs", m: func(t *testing.T) deployments.Model { return mounted(t, starting, shop) },
			keys: tuitest.Keys("l"), want: views.StatusMsg{Level: status.Warn, Text: "deployment starting: 0 running pods"},
		},
		{
			name: "no row under the cursor", m: func(t *testing.T) deployments.Model { return mounted(t, shopClient, shop) },
			keys: on("zzz", tuitest.Press("f")), want: views.StatusMsg{Level: status.Info, Text: "no deployment selected"},
		},
		{
			name: "l on 0 replicas", m: func(t *testing.T) deployments.Model { return mounted(t, shopClient, shop) },
			keys: on("idle", tuitest.Press("l")), want: views.StatusMsg{Level: status.Warn, Text: "deployment idle has 0 replicas"},
		},
		{
			name: "f on 0 replicas", m: func(t *testing.T) deployments.Model { return mounted(t, shopClient, shop) },
			keys: on("idle", tuitest.Press("f")), want: views.StatusMsg{Level: status.Warn, Text: "deployment idle has 0 replicas"},
		},
		{
			name: "l without a selector", m: func(t *testing.T) deployments.Model { return mounted(t, shopClient, shop) },
			keys: on("nosel", tuitest.Press("l")), want: views.StatusMsg{Level: status.Warn, Text: "deployment nosel has no selector"},
		},
		{
			name: "l with only pods of other owners",
			m: func(t *testing.T) deployments.Model {
				return mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
			},
			keys: on(tuitest.DeploymentWeb, tuitest.Press("l")),
			want: views.StatusMsg{Level: status.Warn, Text: "deployment web: 0 running pods · 2 other pods skipped (kubectl logs -l would include them)"},
		},
		{
			name: "l when the lookup fails", m: func(t *testing.T) deployments.Model { return mounted(t, failing, shop) },
			keys: on("api", tuitest.Press("l")),
			want: views.StatusMsg{Level: status.Error, Text: "deployment api: get deployment: etcdserver: request timed out"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, tc.m(t), tc.keys...)
			if got := tuitest.Requests(out); !only(out, tc.want) {
				t.Errorf("want only %+v, got:\n%s", tc.want, tuitest.Describe(got))
			}
		})
	}
}

func TestEIsNotBound(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...)), shop)
	if _, out := tuitest.Drive(t, m, tuitest.Press("e")); len(tuitest.Requests(out)) != 0 {
		t.Errorf("e emitted:\n%s", tuitest.Describe(out))
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
			want: []string{"deployments unavailable: list deployments: deployments.apps is forbidden"},
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

	for _, input := range []tea.Msg{tuitest.Type("mul"), tuitest.Paste("mul")} {
		t.Run(fmt.Sprintf("%T", input), func(t *testing.T) {
			t.Parallel()
			m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
			m, _ = tuitest.Drive(t, m, tuitest.Press("/"), input)
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
		})
	}
}

// TestFilterKeepsTheCursorRow: the cursor stays on its deployment while
// the rows around it come and go.
func TestFilterKeepsTheCursorRow(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...)), shop)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"), tuitest.Press("down"))
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("worker")...)
	_, out := tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Esc(), tuitest.Enter())
	want := views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: "pay-worker"}
	if !only(out, want) {
		t.Errorf("the cursor left pay-worker; enter asked:\n%s", tuitest.Describe(tuitest.Requests(out)))
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
		{namespace: "", want: "kubectl get deployments -A"},
		{namespace: tuitest.NamespaceDemo, want: "kubectl get deployments -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			m := deployments.New(nil, tc.namespace)
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

func TestStaleInstanceReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, deployments.New(c, tuitest.NamespaceDemo, deployments.WithClock(newClock())))
	fresh, _ := tuitest.Drive(t, deployments.New(c, tuitest.NamespaceDemo, deployments.WithClock(newClock())), tuitest.Resize(width, height))
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if !strings.Contains(tuitest.Screen(fresh), "loading deployments") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
	fresh, _ = tuitest.Init(t, fresh)
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
	if m.Init() != nil {
		t.Error("a view without a client has nothing to list")
	}
	names := tuitest.HelpKeys(m)
	if want := []string{"enter", "l", "f", "/", "w", "r"}; !slices.Equal(names, want) {
		t.Errorf("help keys = %v, want %v", names, want)
	}
}
