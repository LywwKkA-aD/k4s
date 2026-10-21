package command_test

import (
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  command.Target
	}{
		{input: "pods", want: command.Target{Name: "pods"}},
		{input: "  PO  ", want: command.Target{Name: "pods"}},
		{input: "pods demo", want: command.Target{Name: "pods", Namespace: "demo", Scoped: true}},
		{input: "po   demo", want: command.Target{Name: "pods", Namespace: "demo", Scoped: true}},
		{input: "pods -n demo", want: command.Target{Name: "pods", Namespace: "demo", Scoped: true}},
		{input: "pods -A", want: command.Target{Name: "pods", All: true, Scoped: true}},
		{input: "deploy --all-namespaces", want: command.Target{Name: "deployments", All: true, Scoped: true}},
		{input: "svc kube-system", want: command.Target{Name: "services", Namespace: "kube-system", Scoped: true}},
		{input: "top demo", want: command.Target{Name: "top", Namespace: "demo", Scoped: true}},
		{input: "ev demo", want: command.Target{Name: "events", Namespace: "demo", Scoped: true}},
		{input: "events -A", want: command.Target{Name: "events", All: true, Scoped: true}},
		// all is a valid namespace name; every namespace is -A.
		{input: "pods all", want: command.Target{Name: "pods", Namespace: "all", Scoped: true}},
		{input: "ns demo", want: command.Target{Name: "namespaces", Namespace: "demo", Scoped: true}},
		{input: "ns -A", want: command.Target{Name: "namespaces", All: true, Scoped: true}},
		{input: "dashboard", want: command.Target{Name: "dashboard"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := command.Parse(tc.input)
			if err != nil || got != tc.want {
				t.Errorf("Parse(%q) = %+v, %v; want %+v", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		is    error
		text  string
	}{
		{input: "", is: command.ErrUnknown, text: "unknown command: "},
		{input: "nope", is: command.ErrUnknown, text: "unknown command: nope"},
		{input: "dashboard demo", is: command.ErrNoNamespace, text: "dashboard takes no namespace"},
		{input: "ctx demo", is: command.ErrNoNamespace, text: "contexts takes no namespace"},
		// Nodes are cluster-scoped: no namespace, not even every one.
		{input: "nodes demo", is: command.ErrNoNamespace, text: "nodes takes no namespace"},
		{input: "no -A", is: command.ErrNoNamespace, text: "nodes takes no namespace"},
		{input: "pods Demo", is: k8s.ErrInvalidNamespace},
		{input: "pods -n", is: command.ErrBadArgument, text: "pods: -n needs a namespace"},
		{input: "pods a b", is: command.ErrBadArgument, text: `pods takes one namespace, not "a b"`},
		{input: "pods -n a b", is: command.ErrBadArgument, text: `pods takes one namespace, not "-n a b"`},
		{input: "pods --bogus", is: command.ErrBadArgument, text: `pods: unknown flag "--bogus"`},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			_, err := command.Parse(tc.input)
			if !errors.Is(err, tc.is) {
				t.Fatalf("Parse(%q) error = %v, want %v", tc.input, err, tc.is)
			}
			if tc.text != "" && err.Error() != tc.text {
				t.Errorf("Parse(%q) error = %q, want %q", tc.input, err, tc.text)
			}
		})
	}
}

func TestTakesNamespace(t *testing.T) {
	t.Parallel()

	for _, name := range command.All() {
		want := name == "pods" || name == "deployments" || name == "services" || name == "top" || name == "events" || name == "namespaces"
		if got := command.TakesNamespace(name); got != want {
			t.Errorf("TakesNamespace(%q) = %v, want %v", name, got, want)
		}
	}
}
