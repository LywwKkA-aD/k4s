package dashboard_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

func mounted(t *testing.T, c *k8s.Client) dashboard.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, dashboard.New(c, nil), 120, 20)
	return m
}

func deny(resource string) tuitest.FakeOption {
	return tuitest.WithReactor("list", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New(resource + " is forbidden")
	})
}

func TestDashboardStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		m       func(t *testing.T) dashboard.Model
		want    []string
		notWant []string
	}{
		{
			name: "loading",
			m:    func(t *testing.T) dashboard.Model { return dashboard.New(tuitest.NewFakeClient(t), nil) },
			want: []string{"loading cluster stats"},
		},
		{
			name:    "counters",
			m:       func(t *testing.T) dashboard.Model { return mounted(t, tuitest.NewFakeClient(t)) },
			want:    []string{"namespaces 3", "pods 5", "deployments 2", "services 2"},
			notWant: []string{"unavailable"},
		},
		{
			name: "partial failure keeps the other counters",
			m:    func(t *testing.T) dashboard.Model { return mounted(t, tuitest.NewFakeClient(t, deny("pods"))) },
			want: []string{"namespaces 3", "pods 0", "unavailable: pods"},
		},
		{
			name: "every count failing",
			m: func(t *testing.T) dashboard.Model {
				return mounted(t, tuitest.NewFakeClient(t, deny("pods"), deny("namespaces"), deny("deployments"), deny("services")))
			},
			want: []string{"stats unavailable:"},
		},
		{
			name: "no kubeconfig",
			m: func(t *testing.T) dashboard.Model {
				m, _ := tuitest.Mount(t, dashboard.New(nil, nil), 120, 20)
				return m
			},
			want: []string{"no kubeconfig"},
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
	if err := c.Clientset.CoreV1().Pods(tuitest.NamespaceDemo).Delete(context.Background(), tuitest.PodCrash, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete pod: %v", err)
	}
	m, _ = tuitest.Drive(t, m, first...)
	m, third := tuitest.Step(t, m, tuitest.Press("r"))
	if len(third) != 1 {
		t.Fatalf("the reply releases busy:\n%s", tuitest.Describe(third))
	}
	m, _ = tuitest.Drive(t, m, third...)
	if !strings.Contains(tuitest.Screen(m), "pods 4") {
		t.Errorf("refresh did not recount:\n%s", tuitest.Screen(m))
	}
}

func TestStaleGenerationReplyIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	_, oldReplies := tuitest.Init(t, dashboard.New(c, nil))
	fresh, _ := tuitest.Drive(t, dashboard.New(c, nil), oldReplies...)
	if !strings.Contains(tuitest.Screen(fresh), "loading cluster stats") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(fresh))
	}
}

func TestWatchToggleAndContract(t *testing.T) {
	t.Parallel()

	m := dashboard.New(nil, nil)
	if got := m.KubectlEquivalent(); got != "kubectl get all -A --watch" {
		t.Errorf("default = %q", got)
	}
	m, out := tuitest.Drive(t, m, tuitest.Press("w"), tuitest.Press("r"))
	if got := m.KubectlEquivalent(); got != "kubectl get all -A" {
		t.Errorf("after w = %q", got)
	}
	if len(out) != 0 {
		t.Errorf("r without a client fetched:\n%s", tuitest.Describe(out))
	}
	if m.Title() != "dashboard" || m.CapturesKeys() || m.Close() != nil || len(m.Help()) != 2 {
		t.Errorf("contract: title=%q help=%d", m.Title(), len(m.Help()))
	}
}
