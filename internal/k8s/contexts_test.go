package k8s

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListContexts(t *testing.T) {
	t.Parallel()

	path := writeKubeconfig(t, "alpha")
	want := []Context{
		{Name: "alpha", Cluster: "c1", AuthInfo: "u1", Namespace: "team-a", Current: true},
		{Name: "beta", Cluster: "c1", AuthInfo: "u1"},
	}
	cases := []struct {
		name string
		list func() ([]Context, error)
	}{
		{name: "explicit path", list: func() ([]Context, error) { return ListContexts(path) }},
		{name: "flag source", list: func() ([]Context, error) {
			return ListContextsFrom(Source{Paths: []string{path}, Origin: OriginFlag})
		}},
		{name: "kubectl source", list: func() ([]Context, error) {
			return ListContextsFrom(Source{Paths: []string{filepath.Join(t.TempDir(), "absent"), path}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.list()
			if err != nil {
				t.Fatalf("list contexts: %v", err)
			}
			// The kubeconfig map has no order; callers sort, so the test does too.
			slices.SortFunc(got, func(a, b Context) int { return strings.Compare(a.Name, b.Name) })
			if !slices.Equal(got, want) {
				t.Fatalf("contexts = %+v, want %+v", got, want)
			}
		})
	}
}

func TestListContextsUnreadableKubeconfig(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "absent")
	_, err := ListContexts(missing)
	if err == nil || !strings.HasPrefix(err.Error(), "load kubeconfig: ") {
		t.Fatalf("err = %v, want a load kubeconfig error", err)
	}
}

func TestValidateNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ok   bool
	}{
		{name: "default", ok: true},
		{name: "k4s-demo", ok: true},
		{name: "0team", ok: true},
		// A namespace may literally be called "all"; k4s uses -A for all namespaces.
		{name: "all", ok: true},
		{name: strings.Repeat("a", 63), ok: true},
		{name: "", ok: false},
		{name: "Team", ok: false},
		{name: "team_a", ok: false},
		{name: "-team", ok: false},
		{name: "team-", ok: false},
		{name: "a.b", ok: false},
		{name: strings.Repeat("a", 64), ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateNamespace(tc.name)
			if tc.ok != (err == nil) {
				t.Fatalf("ValidateNamespace(%q) = %v, want ok=%v", tc.name, err, tc.ok)
			}
			if err != nil && !errors.Is(err, ErrInvalidNamespace) {
				t.Fatalf("ValidateNamespace(%q) = %v, want ErrInvalidNamespace", tc.name, err)
			}
		})
	}
}
