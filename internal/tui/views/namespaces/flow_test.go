package namespaces_test

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
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
)

const (
	width  = 120
	height = 30
)

func mounted(t *testing.T, c *k8s.Client) namespaces.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, namespaces.New(c), width, height)
	return m
}

func TestEnterSelectsNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []tea.Msg
		want views.NamespaceSelectedMsg
	}{
		{name: "the all row clears the namespace", keys: tuitest.Keys("enter"), want: views.NamespaceSelectedMsg{Namespace: ""}},
		{name: "a namespace row", keys: tuitest.Keys("down", "down", "enter"), want: views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo}},
		{
			name: "the filter keeps the all row on top",
			keys: slices.Concat(tuitest.FilterBy("kube"), tuitest.Keys("down", "enter")),
			want: views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceSystem},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t)), tc.keys...)
			if got := tuitest.Requests(out); len(got) != 1 || !tuitest.Contains(got, tc.want) {
				t.Errorf("want only %+v, got:\n%s", tc.want, tuitest.Describe(out))
			}
		})
	}
}

func TestKeysWithoutAnAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    namespaces.Model
		keys []tea.Msg
	}{
		{name: "l e f are not bound", m: mounted(t, tuitest.NewFakeClient(t)), keys: tuitest.Keys("l", "e", "f")},
		{name: "rows not loaded", m: namespaces.New(tuitest.NewFakeClient(t)), keys: tuitest.Keys("enter")},
		{name: "no kubeconfig", m: namespaces.New(nil), keys: tuitest.Keys("enter", "r")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, out := tuitest.Drive(t, tc.m, tc.keys...); len(out) != 0 {
				t.Errorf("emitted:\n%s", tuitest.Describe(out))
			}
		})
	}
}

func TestViewStates(t *testing.T) {
	t.Parallel()

	failing := tuitest.WithReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("namespaces is forbidden")
	})
	cases := []struct {
		name    string
		m       func(t *testing.T) namespaces.Model
		want    []string
		notWant []string
	}{
		{name: "no kubeconfig", m: func(*testing.T) namespaces.Model { return namespaces.New(nil) }, want: []string{"no kubeconfig"}},
		{
			name: "loading",
			m:    func(t *testing.T) namespaces.Model { return namespaces.New(tuitest.NewFakeClient(t)) },
			want: []string{"loading namespaces"},
		},
		{
			name: "table",
			m:    func(t *testing.T) namespaces.Model { return mounted(t, tuitest.NewFakeClient(t)) },
			want: []string{"<all>", tuitest.NamespaceDefault, tuitest.NamespaceDemo, tuitest.NamespaceSystem, "Active"},
		},
		{
			name: "filter keeps the all row",
			m: func(t *testing.T) namespaces.Model {
				m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t)), tuitest.FilterBy("dem")...)
				return m
			},
			want:    []string{"<all>", tuitest.NamespaceDemo, "/ dem"},
			notWant: []string{tuitest.NamespaceSystem},
		},
		{
			name: "fetch error",
			m:    func(t *testing.T) namespaces.Model { return mounted(t, tuitest.NewFakeClient(t, failing)) },
			want: []string{"namespaces unavailable: list namespaces: namespaces is forbidden"},
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

	m, _ := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t)), tuitest.Press("/"), tuitest.Type("r"))
	if !m.CapturesKeys() {
		t.Fatal("open filter must capture keys")
	}
	m, out := tuitest.Drive(t, m, tuitest.Esc())
	if m.CapturesKeys() || len(out) != 0 {
		t.Errorf("esc closes the filter quietly: captures=%v out=%s", m.CapturesKeys(), tuitest.Describe(out))
	}
}

func TestWatchToggle(t *testing.T) {
	t.Parallel()

	m := namespaces.New(nil)
	if got := m.KubectlEquivalent(); got != "kubectl get namespaces --watch" {
		t.Errorf("default = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := m.KubectlEquivalent(); got != "kubectl get namespaces" {
		t.Errorf("after w = %q", got)
	}
}

func TestRefreshRespectsBusy(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	m := mounted(t, c)
	m, first := tuitest.Step(t, m, tuitest.Press("r"))
	if len(first) != 1 {
		t.Fatalf("r should issue one fetch:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("r"))
	if len(second) != 0 {
		t.Fatalf("r while busy must not fetch again:\n%s", tuitest.Describe(second))
	}
	if err := c.Clientset.CoreV1().Namespaces().Delete(context.Background(), tuitest.NamespaceSystem, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
	m, _ = tuitest.Drive(t, m, first...)
	m, third := tuitest.Step(t, m, tuitest.Press("r"))
	if len(third) != 1 {
		t.Fatalf("the reply releases busy:\n%s", tuitest.Describe(third))
	}
	m, _ = tuitest.Drive(t, m, third...)
	if strings.Contains(tuitest.Screen(m), tuitest.NamespaceSystem) {
		t.Errorf("refresh did not drop the deleted namespace:\n%s", tuitest.Screen(m))
	}
}

func TestStaleGenerationReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, namespaces.New(c))
	fresh, _ := tuitest.Drive(t, namespaces.New(c), tuitest.Resize(width, height))
	fresh, inFlight := tuitest.Step(t, fresh, tuitest.Press("r"))
	fresh, _ = tuitest.Drive(t, fresh, oldReplies...)

	if !strings.Contains(tuitest.Screen(fresh), "loading namespaces") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
	if _, again := tuitest.Step(t, fresh, tuitest.Press("r")); len(again) != 0 {
		t.Errorf("the stale reply released the busy flag:\n%s", tuitest.Describe(again))
	}
	fresh, _ = tuitest.Drive(t, fresh, inFlight...)
	if !strings.Contains(tuitest.Screen(fresh), tuitest.NamespaceDemo) {
		t.Errorf("own reply not applied:\n%s", tuitest.Screen(fresh))
	}
}

func TestStaticContract(t *testing.T) {
	t.Parallel()

	m := namespaces.New(nil)
	if m.Title() != "namespaces" || m.CapturesKeys() || m.Close() != nil {
		t.Errorf("Title=%q CapturesKeys=%v", m.Title(), m.CapturesKeys())
	}
	names := tuitest.HelpKeys(m)
	if want := []string{"enter", "/", "w", "r"}; !slices.Equal(names, want) {
		t.Errorf("help keys = %v, want %v", names, want)
	}
}
