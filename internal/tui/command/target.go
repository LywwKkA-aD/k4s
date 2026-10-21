package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

var (
	// ErrUnknown is an entry that names no command.
	ErrUnknown = errors.New("unknown command")
	// ErrNoNamespace is a namespace given to a command that lists no
	// namespace, such as :dashboard or :ctx.
	ErrNoNamespace = errors.New("takes no namespace")
	// ErrBadArgument is an argument Parse cannot read as one namespace.
	ErrBadArgument = errors.New("bad argument")
)

// Target is a command bar entry: the canonical command and, for the
// commands that take one, the namespace asked for.
type Target struct {
	Name string
	// Namespace is the namespace named; "" with All for every namespace.
	Namespace string
	All       bool
	// Scoped is set when the entry named a namespace or every namespace;
	// without it the command keeps the namespace in use.
	Scoped bool
}

// namespaced are the commands that list one namespace at a time, and ns,
// whose argument switches the namespace in place.
var namespaced = map[string]bool{
	"pods": true, "deployments": true, "services": true, "top": true, "namespaces": true,
}

// TakesNamespace reports whether the canonical command takes a namespace
// argument.
func TakesNamespace(canonical string) bool { return namespaced[canonical] }

// Parse reads a command bar entry: a command or alias, then for pods,
// deployments, services, top and ns an optional namespace as kubectl
// spells it: `demo`, `-n demo`, or `-A` / `--all-namespaces` for every
// namespace. "all" is a namespace name like any other. The name is
// checked as a namespace name only, never looked up: RBAC may forbid
// reading namespaces.
func Parse(input string) (Target, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return Target{}, fmt.Errorf("%w: %s", ErrUnknown, strings.TrimSpace(input))
	}
	name, ok := Resolve(fields[0])
	if !ok {
		return Target{}, fmt.Errorf("%w: %s", ErrUnknown, strings.Join(fields, " "))
	}
	t := Target{Name: name}
	args := fields[1:]
	if len(args) == 0 {
		return t, nil
	}
	if !TakesNamespace(name) {
		return Target{}, fmt.Errorf("%s %w", name, ErrNoNamespace)
	}
	return scoped(t, args)
}

// scoped reads args as the namespace of t.
func scoped(t Target, args []string) (Target, error) {
	t.Scoped = true
	switch {
	case len(args) == 1 && (args[0] == "-A" || args[0] == "--all-namespaces"):
		t.All = true
		return t, nil
	case len(args) == 1 && (args[0] == "-n" || args[0] == "--namespace"):
		return Target{}, badArgument(fmt.Sprintf("%s: %s needs a namespace", t.Name, args[0]))
	case len(args) == 2 && (args[0] == "-n" || args[0] == "--namespace"):
		args = args[1:]
	case len(args) > 1:
		return Target{}, badArgument(fmt.Sprintf("%s takes one namespace, not %q", t.Name, strings.Join(args, " ")))
	case strings.HasPrefix(args[0], "-"):
		return Target{}, badArgument(fmt.Sprintf("%s: unknown flag %q", t.Name, args[0]))
	}
	if err := k8s.ValidateNamespace(args[0]); err != nil {
		return Target{}, err
	}
	t.Namespace = args[0]
	return t, nil
}

// badArgument is an ErrBadArgument worded for the status row, without the
// sentinel's own text in front.
type badArgument string

func (e badArgument) Error() string { return string(e) }

func (badArgument) Is(target error) bool { return target == ErrBadArgument }
