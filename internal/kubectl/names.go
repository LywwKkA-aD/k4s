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

// checkArgs validates c's namespace and container flag values and refuses
// a session flag (CheckFlags). Words after "--" belong to the remote
// command and are left alone.
func checkArgs(c Cmd) error {
	if c.IsZero() {
		return ErrNoCommand
	}
	if err := CheckFlags(c); err != nil {
		return err
	}
	args := c.Args
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		kind, value, inline, ok := nameFlag(args[i])
		if !ok {
			continue
		}
		if !inline {
			if i+1 >= len(args) {
				// Quoted: the word is the flag itself here, but an error
				// never prints a word from Args raw.
				return fmt.Errorf("%w: %q has no value", ErrInvalidName, args[i])
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

// nameFlag returns the kind of name word w sets (see nameFlags) and,
// when w carries it, the value; ok is false for any other word. kubectl
// reads -nX as -n X, so a single-dash word that starts with -n or -c sets
// that flag, whatever follows.
func nameFlag(w string) (kind, value string, inline, ok bool) {
	flag, val, hasVal := strings.Cut(w, "=")
	if k, found := nameFlags[flag]; found {
		return k, val, hasVal, true
	}
	if len(w) > 2 && w[0] == '-' && (w[1] == 'n' || w[1] == 'c') {
		return nameFlags[w[:2]], w[2:], true, true
	}
	return "", "", false, false
}
