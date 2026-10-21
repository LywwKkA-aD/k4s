package dash

import (
	"slices"
	"strings"
)

// Cmd is one read-only kubectl invocation. Args follow the binary name, so
// Cmd{Args: []string{"get", "pods", "-A"}} is `kubectl get pods -A`.
// Approx marks content k4s derived or summarised from that command's output
// (rendered with ≈); the command itself is still exact.
type Cmd struct {
	Args   []string
	Approx bool
}

// Argv returns the full argv with the binary first, in a fresh slice.
func (c Cmd) Argv() []string {
	return append([]string{"kubectl"}, c.Args...)
}

// String renders the command for a POSIX shell: arguments that carry shell
// metacharacters are single-quoted.
func (c Cmd) String() string {
	parts := make([]string, 0, len(c.Args)+1)
	for _, a := range c.Argv() {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// IsZero reports whether c names no command at all.
func (c Cmd) IsZero() bool { return len(c.Args) == 0 }

// readOnlyVerbs is the dashboard's verb allow-list; nothing the dashboard
// suggests may change the cluster.
var readOnlyVerbs = []string{"get", "describe", "logs", "top", "events", "version", "cluster-info", "api-resources"}

// ReadOnly reports whether c uses a verb on the dashboard allow-list. auth
// is allowed only for its read-only subcommands.
func ReadOnly(c Cmd) bool {
	if c.IsZero() {
		return false
	}
	if c.Args[0] == "auth" {
		return len(c.Args) > 1 && (c.Args[1] == "can-i" || c.Args[1] == "whoami")
	}
	return slices.Contains(readOnlyVerbs, c.Args[0])
}

func kubectl(args ...string) Cmd { return Cmd{Args: args} }

func approx(c Cmd) Cmd {
	return Cmd{Args: c.Args, Approx: true}
}

// scopeArgs is -A for all namespaces, else -n <ns>.
func scopeArgs(ns string) []string {
	if ns == "" {
		return []string{"-A"}
	}
	return []string{"-n", ns}
}

// withScope puts -A or -n <ns> before the first flag, where kubectl users
// write it: `kubectl get svc -A --field-selector …`.
func withScope(ns string, args ...string) Cmd {
	at := slices.IndexFunc(args, func(a string) bool { return strings.HasPrefix(a, "-") })
	if at < 0 {
		at = len(args)
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[:at]...)
	out = append(out, scopeArgs(ns)...)
	return Cmd{Args: append(out, args[at:]...)}
}

// canIList is the probe a denied list points at. namespaced resources get
// -A or -n so the probe asks exactly what the dashboard asked.
func canIList(resource, ns string, namespaced bool) Cmd {
	if !namespaced {
		return kubectl("auth", "can-i", "list", resource)
	}
	return withScope(ns, "auth", "can-i", "list", resource)
}

func describe(kind, name, ns string) Cmd {
	if ns == "" {
		return kubectl("describe", kind, name)
	}
	return kubectl("describe", kind, name, "-n", ns)
}

// shellQuote leaves words made only of safe characters bare and
// single-quotes the rest, closing and reopening around embedded quotes.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, unsafeShellRune) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func unsafeShellRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	case strings.ContainsRune("_@%+=:,./-", r):
		return false
	default:
		return true
	}
}
