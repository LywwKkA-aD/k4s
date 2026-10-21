// Package kubectl models the kubectl command behind every k4s action: the
// footer shows it, `y` copies it, and exec and describe run exactly its
// argv through a Runner, never through a shell.
package kubectl

import (
	"path/filepath"
	"slices"
	"strings"
)

const (
	// Name is the program every Cmd renders as its first word.
	Name = "kubectl"
	// ApproxMark prefixes the displayed command when k4s does a little more
	// or less than the command would, so it is never copied.
	ApproxMark = "≈"
	// EnvKubeconfig is the variable kubectl reads its kubeconfig list from.
	EnvKubeconfig = "KUBECONFIG"
)

// Cmd is one kubectl invocation. Values are immutable: the With methods
// return a new Cmd and never share slices with their receiver or caller.
//
// Args are the words after "kubectl". They must not carry --context or
// --kubeconfig; WithContext and WithKubeconfig (or Decorate) add those so
// that every command places them the same way.
type Cmd struct {
	Args []string
	// Mutating marks commands that change cluster state, for a confirm step.
	Mutating bool
	// Approx marks a command that matches what k4s does only approximately,
	// for example when k4s skips finished pods that `kubectl logs -l` would
	// stream. Display prefixes ApproxMark; String never does.
	Approx bool

	context    string
	kubeconfig []string
}

// New returns a Cmd running kubectl with args.
func New(args ...string) Cmd {
	return Cmd{Args: slices.Clone(args)}
}

// IsZero reports whether c has nothing to run.
func (c Cmd) IsZero() bool { return len(c.Args) == 0 }

// Argv returns the program and every argument, global flags first, as a
// fresh slice; nil for a zero Cmd. Env assignments are not part of it.
func (c Cmd) Argv() []string {
	if c.IsZero() {
		return nil
	}
	out := make([]string, 0, 5+len(c.Args))
	out = append(out, Name)
	if len(c.kubeconfig) == 1 {
		out = append(out, "--kubeconfig", c.kubeconfig[0])
	}
	if c.context != "" {
		out = append(out, "--context", c.context)
	}
	return append(out, c.Args...)
}

// Env returns the NAME=value assignments the command needs in front of it:
// KUBECONFIG when it was pinned to several files, which --kubeconfig cannot
// express. Nil when there are none.
func (c Cmd) Env() []string {
	if len(c.kubeconfig) < 2 {
		return nil
	}
	return []string{EnvKubeconfig + "=" + strings.Join(c.kubeconfig, string(filepath.ListSeparator))}
}

// String renders the command as one POSIX shell line that, pasted, runs
// exactly Argv with Env. Words are quoted only where a shell would change
// them. A zero Cmd renders as "".
func (c Cmd) String() string {
	argv := c.Argv()
	if argv == nil {
		return ""
	}
	env := c.Env()
	words := make([]string, 0, len(env)+len(argv))
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		words = append(words, name+"="+Quote(value))
	}
	for _, a := range argv {
		words = append(words, Quote(a))
	}
	return strings.Join(words, " ")
}

// Display is String with ApproxMark in front when c is Approx: the footer
// form. Copy String, not Display.
func (c Cmd) Display() string {
	s := c.String()
	if c.Approx && s != "" {
		return ApproxMark + " " + s
	}
	return s
}

// WithContext returns c with --context name; an empty name removes it.
func (c Cmd) WithContext(name string) Cmd {
	out := c.clone()
	out.context = name
	return out
}

// WithKubeconfig returns c pinned to paths: --kubeconfig for one file, a
// KUBECONFIG assignment for several. Paths are cleaned and empty ones
// dropped; no paths removes the pin.
func (c Cmd) WithKubeconfig(paths []string) Cmd {
	out := c.clone()
	out.kubeconfig = cleanPaths(paths)
	return out
}

func (c Cmd) clone() Cmd {
	out := c
	out.Args = slices.Clone(c.Args)
	out.kubeconfig = slices.Clone(c.kubeconfig)
	return out
}

// cleanPaths returns paths cleaned, without empty entries; nil when none
// remain, so equal lists compare equal.
func cleanPaths(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		out = append(out, filepath.Clean(p))
	}
	return out
}
