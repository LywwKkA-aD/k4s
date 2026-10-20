package pods_test

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
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

const (
	width  = 140
	height = 30
)

func mounted(t *testing.T, c *k8s.Client, namespace string) pods.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, pods.New(c, namespace), width, height)
	return m
}

func keys(prefix []tea.Msg, more ...tea.Msg) []tea.Msg {
	return slices.Concat(prefix, more)
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
			name:      "enter describes the pod",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodWeb1), tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "pod", Namespace: demo, Name: tuitest.PodWeb1},
		},
		{
			name:      "enter in all namespaces reads the namespace column",
			namespace: "",
			keys:      keys(tuitest.FilterBy(tuitest.PodCoreDNS), tuitest.Enter()),
			want:      views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceSystem, Name: tuitest.PodCoreDNS},
		},
		{
			name:      "l on a single-container pod goes straight to the tail prompt",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodWeb1), tuitest.Press("l")),
			want:      views.TailPromptRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}, Container: tuitest.ContainerNginx},
		},
		{
			name:      "l on a multi-container pod asks for the container",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodMulti), tuitest.Press("l")),
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "logs",
			},
		},
		{
			name:      "e on a single-container pod execs straight away",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodWeb1), tuitest.Press("e")),
			want:      views.ExecRequestMsg{Namespace: demo, Pod: tuitest.PodWeb1, Container: tuitest.ContainerNginx},
		},
		{
			name:      "e on a multi-container pod asks for the container",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodMulti), tuitest.Press("e")),
			want: views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "exec",
			},
		},
		{
			name:      "f asks for a port-forward without a port hint",
			namespace: demo,
			keys:      keys(tuitest.FilterBy(tuitest.PodWeb1), tuitest.Press("f")),
			want:      views.ForwardRequestMsg{Kind: "pod", Namespace: demo, Name: tuitest.PodWeb1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := mounted(t, tuitest.NewFakeClient(t), tc.namespace)
			_, out := tuitest.Drive(t, m, tc.keys...)
			if !tuitest.Contains(out, tc.want) {
				t.Errorf("want %T %+v among:\n%s", tc.want, tc.want, tuitest.Describe(out))
			}
		})
	}
}

func TestRowActionsNeedLoadedRows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    pods.Model
	}{
		{name: "not loaded yet", m: pods.New(tuitest.NewFakeClient(t), tuitest.NamespaceDemo)},
		{name: "no kubeconfig", m: pods.New(nil, tuitest.NamespaceDemo)},
		{name: "empty cluster", m: mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()), tuitest.NamespaceDemo)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, tc.m, tuitest.Keys("enter", "l", "e", "f")...)
			if len(out) != 0 {
				t.Errorf("row actions without rows emitted:\n%s", tuitest.Describe(out))
			}
		})
	}
}

func TestViewStates(t *testing.T) {
	t.Parallel()

	failing := tuitest.WithReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("pods is forbidden")
	})
	cases := []struct {
		name    string
		m       func(t *testing.T) pods.Model
		want    []string
		notWant []string
	}{
		{
			name: "no kubeconfig",
			m:    func(*testing.T) pods.Model { return pods.New(nil, "") },
			want: []string{"no kubeconfig"},
		},
		{
			name: "loading",
			m:    func(t *testing.T) pods.Model { return pods.New(tuitest.NewFakeClient(t), "") },
			want: []string{"loading pods"},
		},
		{
			name:    "namespace scoped table",
			m:       func(t *testing.T) pods.Model { return mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo) },
			want:    []string{"NAME", "READY", "STATUS", tuitest.PodWeb1, tuitest.PodMulti, "2/2", "CrashLoopBackOff", "7"},
			notWant: []string{"NAMESPACE", tuitest.PodCoreDNS},
		},
		{
			name: "all namespaces adds the namespace column",
			m:    func(t *testing.T) pods.Model { return mounted(t, tuitest.NewFakeClient(t), "") },
			want: []string{"NAMESPACE", tuitest.NamespaceSystem, tuitest.PodCoreDNS},
		},
		{
			name: "empty",
			m: func(t *testing.T) pods.Model {
				return mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects()), "")
			},
			want: []string{"no pods"},
		},
		{
			name: "fetch error",
			m:    func(t *testing.T) pods.Model { return mounted(t, tuitest.NewFakeClient(t, failing), "") },
			want: []string{"pods unavailable: list pods: pods is forbidden"},
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

func TestFilterCapturesKeysAndNarrowsRows(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	if m.CapturesKeys() {
		t.Fatal("view captures keys before the filter opens")
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("web"))
	if !m.CapturesKeys() {
		t.Fatal("open filter must capture keys")
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "/ web") || !strings.Contains(screen, tuitest.PodWeb2) || strings.Contains(screen, tuitest.PodCrash) {
		t.Errorf("typing narrows the rows live:\n%s", screen)
	}

	// q is text while the prompt is open, not a binding.
	m, out := tuitest.Drive(t, m, tuitest.Press("q"))
	if len(out) != 0 || !strings.Contains(tuitest.Screen(m), "no pods match /webq") {
		t.Errorf("q inside the filter: out=%s screen:\n%s", tuitest.Describe(out), tuitest.Screen(m))
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("backspace"), tuitest.Enter())
	if m.CapturesKeys() {
		t.Error("enter commits the filter and releases the keyboard")
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ web") || strings.Contains(screen, tuitest.PodCrash) {
		t.Errorf("committed filter keeps narrowing:\n%s", screen)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Esc())
	if screen := tuitest.Screen(m); !strings.Contains(screen, tuitest.PodCrash) || strings.Contains(screen, "/ web") {
		t.Errorf("esc clears the filter:\n%s", screen)
	}
}

func TestWatchToggleShowsInKubectlEquivalent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace string
		watching  string
		paused    string
	}{
		{namespace: "", watching: "kubectl get pods -A --watch", paused: "kubectl get pods -A"},
		{namespace: tuitest.NamespaceDemo, watching: "kubectl get pods -n demo --watch", paused: "kubectl get pods -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.watching, func(t *testing.T) {
			t.Parallel()
			m := pods.New(nil, tc.namespace)
			if got := m.KubectlEquivalent(); got != tc.watching {
				t.Errorf("default = %q, want %q", got, tc.watching)
			}
			m, out := tuitest.Drive(t, m, tuitest.Press("w"))
			if got := m.KubectlEquivalent(); got != tc.paused || len(out) != 0 {
				t.Errorf("after w = %q (out %s), want %q", got, tuitest.Describe(out), tc.paused)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
			if got := m.KubectlEquivalent(); got != tc.watching {
				t.Errorf("after second w = %q, want %q", got, tc.watching)
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
		t.Fatalf("r should issue one fetch, got:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("r"))
	if len(second) != 0 {
		t.Fatalf("r while a fetch is in flight must not issue another:\n%s", tuitest.Describe(second))
	}

	if err := c.Clientset.CoreV1().Pods(tuitest.NamespaceDemo).Delete(context.Background(), tuitest.PodWeb2, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete pod: %v", err)
	}
	m, _ = tuitest.Drive(t, m, first...)
	_, third := tuitest.Step(t, m, tuitest.Press("r"))
	if len(third) != 1 {
		t.Errorf("the reply releases busy, so r fetches again; got:\n%s", tuitest.Describe(third))
	}

	m, _ = tuitest.Drive(t, m, third...)
	if screen := tuitest.Screen(m); strings.Contains(screen, tuitest.PodWeb2) {
		t.Errorf("refresh did not pick up the deleted pod:\n%s", screen)
	}
}

func TestStaleGenerationReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, pods.New(c, tuitest.NamespaceDemo))
	if len(oldReplies) == 0 {
		t.Fatal("the first view produced no fetch reply")
	}

	fresh, _ := tuitest.Drive(t, pods.New(c, tuitest.NamespaceDemo), tuitest.Resize(width, height))
	fresh, inFlight := tuitest.Step(t, fresh, tuitest.Press("r"))
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if screen := tuitest.Screen(fresh); !strings.Contains(screen, "loading pods") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", screen)
	}
	if _, again := tuitest.Step(t, fresh, tuitest.Press("r")); len(again) != 0 {
		t.Errorf("the stale reply released the busy flag:\n%s", tuitest.Describe(again))
	}

	fresh, _ = tuitest.Drive(t, fresh, inFlight...)
	if screen := tuitest.Screen(fresh); !strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("the view's own reply was not applied:\n%s", screen)
	}
}

func TestStaticContract(t *testing.T) {
	t.Parallel()

	m := pods.New(nil, "")
	if m.Title() != "pods" {
		t.Errorf("Title = %q", m.Title())
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
	names := tuitest.HelpKeys(m)
	if want := []string{"enter", "l", "e", "f", "/", "w", "r"}; !slices.Equal(names, want) {
		t.Errorf("help keys = %v, want %v", names, want)
	}
}
