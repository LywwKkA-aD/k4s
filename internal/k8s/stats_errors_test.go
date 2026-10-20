package k8s_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

var allResources = []string{"deployments", "namespaces", "pods", "services"}

// TestStatsPartialFailures pins how the dashboard counts degrade when some
// count queries are denied: the rest still count and no error is returned.
// clusterObjects holds 2 of each kind.
func TestStatsPartialFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		deny       []string
		wantFailed []string
		want       k8s.Stats
	}{
		{name: "nothing denied", want: k8s.Stats{Namespaces: 2, Pods: 2, Deployments: 2, Services: 2}},
		{
			name: "deployments denied", deny: []string{"deployments"}, wantFailed: []string{"deployments"},
			want: k8s.Stats{Namespaces: 2, Pods: 2, Services: 2},
		},
		{
			name: "namespaces denied", deny: []string{"namespaces"}, wantFailed: []string{"namespaces"},
			want: k8s.Stats{Pods: 2, Deployments: 2, Services: 2},
		},
		{
			name: "three of four denied", deny: []string{"namespaces", "pods", "services"},
			wantFailed: []string{"namespaces", "pods", "services"},
			want:       k8s.Stats{Deployments: 2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(clusterObjects()...)
			for _, r := range tc.deny {
				k8stest.Forbid(cs, "list", r)
			}

			got, err := c.Stats(context.Background())
			if err != nil {
				t.Fatalf("Stats() = %v; a partial failure must not error", err)
			}
			assertFailed(t, got, tc.wantFailed)
			if counts(got) != counts(tc.want) {
				t.Fatalf("Stats() counts = %v, want %v", counts(got), counts(tc.want))
			}
		})
	}
}

// TestStatsTotalFailures pins the all-four-failed path: zero counts, every
// resource in Failed, and a joined error from which the API reason is still
// recoverable.
func TestStatsTotalFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		install func(*fake.Clientset)
		is      func(error) bool
	}{
		{
			// Today a user limited to one namespace gets no dashboard at
			// all: every count query is cluster-scoped.
			name:    "namespace-scoped RBAC",
			install: func(cs *fake.Clientset) { k8stest.RestrictToNamespace(cs, granted) },
			is:      apierrors.IsForbidden,
		},
		{
			name:    "revoked credentials",
			install: func(cs *fake.Clientset) { k8stest.Unauthorized(cs) },
			is:      apierrors.IsUnauthorized,
		},
		{
			name:    "apiserver slower than the deadline",
			install: func(cs *fake.Clientset) { k8stest.SlowerThanDeadline(cs, 20*time.Millisecond) },
			is:      func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(clusterObjects()...)
			tc.install(cs)

			got, err := c.Stats(context.Background())
			if err == nil {
				t.Fatal("Stats() = nil error with every query failing")
			}
			if !tc.is(err) {
				t.Fatalf("Stats() err = %v, want the API reason reachable through the join", err)
			}
			for _, r := range allResources {
				if !strings.Contains(err.Error(), r+": ") {
					t.Errorf("Stats() err = %q, want a %q-prefixed entry", err, r+": ")
				}
			}
			assertFailed(t, got, allResources)
			if counts(got) != counts(k8s.Stats{}) {
				t.Fatalf("Stats() counts = %v, want all zero", counts(got))
			}
		})
	}
}

func TestStatsHonoursTheCallerDeadlineOverREST(t *testing.T) {
	t.Parallel()

	c := k8stest.NewServerClient(t, k8stest.Hang())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	got, err := c.Stats(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stats() err = %v, want context.DeadlineExceeded", err)
	}
	assertFailed(t, got, allResources)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Stats() outlived its deadline: %v", elapsed)
	}
}

// counts is Stats without Failed, in a comparable form.
func counts(s k8s.Stats) [4]int {
	return [4]int{s.Namespaces, s.Pods, s.Deployments, s.Services}
}

// assertFailed compares Stats.Failed ignoring order: the four queries run
// concurrently and append in completion order.
func assertFailed(t *testing.T, got k8s.Stats, want []string) {
	t.Helper()
	failed := slices.Sorted(slices.Values(got.Failed))
	if len(failed) == 0 && len(want) == 0 {
		return
	}
	if !slices.Equal(failed, want) {
		t.Fatalf("Stats().Failed = %v, want %v", got.Failed, want)
	}
}
