package kubectl

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

var (
	// ErrInvalidName is wrapped by every name check failure.
	ErrInvalidName = errors.New("invalid name")
	// ErrNoCommand is returned for an empty Cmd or an exec with no command.
	ErrNoCommand = errors.New("no command")
)

// CheckLabel reports whether name is a DNS-1123 label, the rule for
// namespaces and container names. kind names the value in the error.
func CheckLabel(kind, name string) error {
	return checkName(kind, name, validation.IsDNS1123Label(name))
}

// CheckSubdomain reports whether name is a DNS-1123 subdomain, the rule for
// pods, deployments and most other object names.
func CheckSubdomain(kind, name string) error {
	return checkName(kind, name, validation.IsDNS1123Subdomain(name))
}

func checkName(kind, name string, problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s %q: %s", ErrInvalidName, kind, name, problems[0])
}

// nameFlags are the flags whose values the Runner checks before running:
// they reach kubectl from API objects and must never read as a flag.
var nameFlags = map[string]string{
	"-n":          "namespace",
	"--namespace": "namespace",
	"-c":          "container",
	"--container": "container",
}

// checkArgs validates c's namespace and container flag values. Words after
// "--" belong to the remote command and are left alone.
func checkArgs(c Cmd) error {
	if c.IsZero() {
		return ErrNoCommand
	}
	args := c.Args
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		flag, value, inline := strings.Cut(args[i], "=")
		kind, ok := nameFlags[flag]
		if !ok {
			continue
		}
		if !inline {
			if i+1 >= len(args) {
				return fmt.Errorf("%w: %s has no value", ErrInvalidName, flag)
			}
			i++
			value = args[i]
		}
		if err := CheckLabel(kind, value); err != nil {
			return err
		}
	}
	return nil
}
