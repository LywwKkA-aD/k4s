package cli

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// envNoPreflight is the environment switch behind --no-preflight.
const envNoPreflight = "K4S_NO_PREFLIGHT"

// themes are the --theme values; styles.ParseMode accepts the same set
// from K4S_THEME (a test in cmd/k4s keeps the two in step).
var themes = []string{"auto", "dark", "light", "mono"}

// Themes lists the accepted --theme values.
func Themes() []string { return slices.Clone(themes) }

// newFlagSet binds every flag to o. Defaults already in o (from the
// environment) stay unless the flag is given.
func newFlagSet(o *Options, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("k4s", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Help goes to stdout and errors to stderr, so usage is printed by
	// Parse rather than by the flag package.
	fs.Usage = func() {}

	fs.StringVar(&o.Kubeconfig, "kubeconfig", "", "kubeconfig file")
	fs.StringVar(&o.Context, "context", "", "kubeconfig context")
	ns := namespaceValue{&o.Namespace}
	fs.Var(ns, "n", "starting namespace")
	fs.Var(ns, "namespace", "starting namespace")
	fs.BoolVar(&o.AllNamespaces, "A", false, "start in all namespaces")
	fs.BoolVar(&o.AllNamespaces, "all-namespaces", false, "start in all namespaces")
	fs.Var(themeValue{&o.Theme}, "theme", "colour theme")
	fs.BoolVar(&o.NoPreflight, "no-preflight", o.NoPreflight, "skip the credential plugin preflight")
	return fs
}

// namespaceValue accepts DNS-1123 labels; "" means not given.
type namespaceValue struct{ dst *string }

func (v namespaceValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

func (v namespaceValue) Set(s string) error {
	if s != "" {
		if err := k8s.ValidateNamespace(s); err != nil {
			return err
		}
	}
	*v.dst = s
	return nil
}

// themeValue accepts one of themes, ignoring case and surrounding space
// like K4S_THEME does.
type themeValue struct{ dst *string }

func (v themeValue) String() string {
	if v.dst == nil {
		return ""
	}
	return *v.dst
}

func (v themeValue) Set(s string) error {
	mode := strings.ToLower(strings.TrimSpace(s))
	if !slices.Contains(themes, mode) {
		return fmt.Errorf("want %s or %s", strings.Join(themes[:len(themes)-1], ", "), themes[len(themes)-1])
	}
	*v.dst = mode
	return nil
}

// envSwitch reads an on/off environment variable: unset, empty, 0, false,
// no and off mean off; anything else means on.
func envSwitch(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}
