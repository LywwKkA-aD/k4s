package tui

import (
	"path/filepath"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/startup"
)

// WithSession hands the root what startup found besides the client: the
// kubeconfig Source, which the contexts picker and :ctx read when there
// is no client, the distribution and server version, and the preflight
// error, shown in the status row once the TUI starts. Session.Client and
// Session.LoadErr are New's own arguments and are not read from here.
func WithSession(s startup.Session) Option {
	return func(m *Model) {
		s.Client, s.LoadErr = nil, nil
		m.startup = s
	}
}

// WithFlags records which of --context and --kubeconfig the user passed.
// A cluster asked for by name stays named in every footer command, even
// where a bare kubectl would reach it anyway.
func WithFlags(o cli.Options) Option {
	return func(m *Model) {
		m.flags = sessionFlags{context: o.Context != "", kubeconfig: o.Kubeconfig != ""}
	}
}

// sessionFlags are the global kubectl flags the user started k4s with.
type sessionFlags struct {
	context    bool
	kubeconfig bool
}

// newKubectlSession describes for kubectl.Decorate how k4s reached c and
// where a bare kubectl in the same shell would go: getenv and home are
// that shell's environment and home directory. Both kubeconfig lists are
// made absolute, so a pasted command works from any directory and the two
// compare equal when they name the same files.
//
// This is the one rule for --kubeconfig, and it agrees with
// k8s.Source.KubeconfigArg: a file named with --kubeconfig and the k3s
// server's own file are pinned, the files a bare kubectl loads
// ($KUBECONFIG, ~/.kube/config) are not.
func newKubectlSession(c *k8s.Client, flags sessionFlags, getenv func(string) string, home string) kubectl.Session {
	if c == nil {
		return kubectl.Session{}
	}
	return kubectl.Session{
		Context:        c.Context,
		CurrentContext: c.CurrentContext,
		ContextFlag:    flags.context,
		Kubeconfig:     absPaths(c.KubeconfigPaths),
		BareKubeconfig: absPaths(kubectl.BareKubeconfig(getenv(kubectl.EnvKubeconfig), home)),
		KubeconfigFlag: flags.kubeconfig || c.Source.Origin == k8s.OriginFlag,
	}
}

// absPaths returns paths made absolute, in a new slice; a path that
// cannot be resolved is kept as it is.
func absPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		out = append(out, p)
	}
	return out
}

// kubeconfigSource is where the session's kubeconfig was looked up: the
// client's, else what startup found when loading failed.
func (m Model) kubeconfigSource() k8s.Source {
	if m.client != nil {
		return m.client.Source
	}
	return m.startup.Source
}

// decorate adds the global flags a pasted copy of c needs to reach the
// cluster k4s shows.
func (m Model) decorate(c kubectl.Cmd) kubectl.Cmd {
	return kubectl.Decorate(c, m.kubectlSession)
}

// preflightText is the status line for a failed preflight.
func preflightText(err error) string {
	text := err.Error()
	if strings.HasPrefix(text, "preflight:") {
		return text
	}
	return "preflight: " + text
}

// clientNamespace is where c starts, "" (all namespaces) without a client.
func clientNamespace(c *k8s.Client) string {
	if c == nil {
		return ""
	}
	return c.Namespace
}
