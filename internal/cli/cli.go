// Package cli parses the k4s command line. It runs before anything touches
// the terminal or a cluster, so --version and --help work anywhere.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/version"
)

// ExitUsage is the exit code for a bad command line, matching the flag
// package's own convention.
const ExitUsage = 2

const usage = `k4s is a terminal UI for Kubernetes (k8s and k3s) that shows the kubectl equivalent of every action.

Usage:
  k4s [flags]

Flags:
      --kubeconfig <file>   kubeconfig to use instead of $KUBECONFIG, ~/.kube/config
                            and, on a k3s server, /etc/rancher/k3s/k3s.yaml
      --context <name>      kubeconfig context to use instead of its current-context
  -n, --namespace <name>    namespace to start in (default: the context's namespace,
                            or all namespaces when it sets none)
  -A, --all-namespaces      start in all namespaces
      --theme <mode>        colour theme: auto, dark, light, mono or high-contrast
                            (text at 7:1); wins over K4S_THEME and NO_COLOR
      --no-preflight        do not run the credential plugin (SSO, MFA) on the plain
                            terminal before the TUI starts
      --no-recap            do not print the session's kubectl commands on exit
  -h, --help                show this help and exit
      --version             print the version and exit

Environment:
  KUBECONFIG         kubeconfig file list to load (default ~/.kube/config)
  K4S_THEME          colour theme: auto, dark, light, mono or high-contrast
                     (default auto)
  K4S_NO_PREFLIGHT   when set to anything but 0 or false, same as --no-preflight
  K4S_NO_RECAP       when set to anything but 0 or false, same as --no-recap
  NO_COLOR           when set, disable colour (same as K4S_THEME=mono)
`

// Options is the parsed command line. An empty string means the flag was
// not given.
type Options struct {
	// Kubeconfig replaces $KUBECONFIG and the default kubeconfig search.
	Kubeconfig string
	// Context replaces the kubeconfig's current-context.
	Context string
	// Namespace is a valid DNS-1123 label.
	Namespace string
	// AllNamespaces wins over Namespace, as -A does in kubectl.
	AllNamespaces bool
	// Theme is one of Themes, lower-cased.
	Theme string
	// NoPreflight is --no-preflight, or K4S_NO_PREFLIGHT when the flag is
	// not given.
	NoPreflight bool
	// NoRecap is --no-recap, or K4S_NO_RECAP when the flag is not given:
	// no kubectl commands are printed on exit.
	NoRecap bool
}

// Parse handles args (the command line without the program name), reading
// K4S_NO_PREFLIGHT and K4S_NO_RECAP through getenv. It returns proceed=true when the TUI
// should start. Otherwise the caller must exit with code; any version, help
// or error text has already been written. Stray positional arguments are
// ignored, as in earlier releases.
func Parse(args []string, getenv func(string) string, stdout, stderr io.Writer) (opts Options, proceed bool, code int) {
	opts = Options{
		NoPreflight: envSwitch(getenv(envNoPreflight)),
		NoRecap:     envSwitch(getenv(envNoRecap)),
	}
	fs := newFlagSet(&opts, stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")

	err := parseInterspersed(fs, args)
	var w io.Writer
	var msg string
	switch {
	case errors.Is(err, flag.ErrHelp):
		w, msg, code = stdout, usage, 0
	case err != nil:
		// The flag package has already written the error line itself.
		w, msg, code = stderr, "\n"+usage, ExitUsage
	case *showVersion:
		w, msg, code = stdout, versionLine()+"\n", 0
	default:
		return opts, true, 0
	}
	// A failed write to stderr changes nothing, but `k4s --version` into a
	// closed pipe must not report success.
	if _, werr := io.WriteString(w, msg); werr != nil && code == 0 {
		return opts, false, 1
	}
	return opts, false, code
}

// Getenv layers the command line over base: --theme stands in for
// K4S_THEME and hides NO_COLOR, since an explicit flag beats both
// (https://no-color.org). Pass the result to the theme setup.
func (o Options) Getenv(base func(string) string) func(string) string {
	if o.Theme == "" {
		return base
	}
	theme := o.Theme
	return func(key string) string {
		switch key {
		case "K4S_THEME":
			return theme
		case "NO_COLOR":
			return ""
		}
		return base(key)
	}
}

// parseInterspersed lets flags follow a stray positional argument, as in
// `k4s pods -n web`: the flag package stops at the first one and would
// silently drop -n. Everything after "--" stays positional.
func parseInterspersed(fs *flag.FlagSet, args []string) error {
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		rest := fs.Args()
		consumed := len(args) - len(rest)
		if len(rest) == 0 || endsWithTerminator(fs, args[:consumed]) {
			return nil
		}
		args = rest[1:]
	}
}

// endsWithTerminator reports whether consumed, the arguments the flag package
// took, ends in a real "--". A "--" that is the value of a flag, as in
// `--kubeconfig --`, is not one.
func endsWithTerminator(fs *flag.FlagSet, consumed []string) bool {
	for i := 0; i < len(consumed); i++ {
		arg := consumed[i]
		if arg == "--" {
			return i == len(consumed)-1
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if hasValue {
			continue
		}
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) {
			i++
		}
	}
	return false
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func versionLine() string {
	return fmt.Sprintf("k4s %s (%s %s/%s)", version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
