// Package cli parses the k4s command line. It runs before anything touches
// the terminal or a cluster, so --version and --help work anywhere.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"

	"github.com/LywwKkA-aD/k4s/internal/version"
)

// ExitUsage is the exit code for a bad command line, matching the flag
// package's own convention.
const ExitUsage = 2

const usage = `k4s is a terminal UI for Kubernetes (k8s and k3s) that shows the kubectl equivalent of every action.

Usage:
  k4s [flags]

Flags:
  -h, --help      show this help and exit
      --version   print the version and exit

Environment:
  KUBECONFIG   kubeconfig file list to load (default ~/.kube/config)
  K4S_THEME    colour theme: auto, dark, light or mono (default auto)
  NO_COLOR     when set, disable colour (same as K4S_THEME=mono)
`

// Parse handles args (the command line without the program name). It returns
// proceed=true when the TUI should start. Otherwise the caller must exit with
// code; any version, help or error text has already been written.
func Parse(args []string, stdout, stderr io.Writer) (proceed bool, code int) {
	fs := flag.NewFlagSet("k4s", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Help goes to stdout and errors to stderr, so usage is printed here
	// rather than by the flag package.
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print the version and exit")

	err := fs.Parse(args)
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
		return true, 0
	}
	// A failed write to stderr changes nothing, but `k4s --version` into a
	// closed pipe must not report success.
	if _, werr := io.WriteString(w, msg); werr != nil && code == 0 {
		return false, 1
	}
	return false, code
}

func versionLine() string {
	return fmt.Sprintf("k4s %s (%s %s/%s)", version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
