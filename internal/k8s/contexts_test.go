package k8s

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListContexts(t *testing.T) {
	t.Parallel()

	got, err := ListContexts(writeKubeconfig(t, "alpha"))
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	// The kubeconfig map has no order; callers sort, so the test does too.
	slices.SortFunc(got, func(a, b Context) int { return strings.Compare(a.Name, b.Name) })
	want := []Context{
		{Name: "alpha", Cluster: "c1", AuthInfo: "u1", Namespace: "team-a", Current: true},
		{Name: "beta", Cluster: "c1", AuthInfo: "u1"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ListContexts = %+v, want %+v", got, want)
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
