package actions

import (
	"context"
	"testing"
)

func TestStaticAdvertisesKubectlEquivalent(t *testing.T) {
	t.Parallel()

	a := NewStatic("list pods", "kubectl get pods -A")

	if got, want := a.Name(), "list pods"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if got, want := a.KubectlEquivalent(), "kubectl get pods -A"; got != want {
		t.Errorf("KubectlEquivalent() = %q, want %q", got, want)
	}
	if err := a.Execute(context.Background()); err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
}

// staticImplementsAction is a compile-time guarantee that Static satisfies
// the Action interface.
var _ Action = Static{}
