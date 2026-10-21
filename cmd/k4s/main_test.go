package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestRunError(t *testing.T) {
	t.Parallel()

	noTTY := &fs.PathError{Op: "open", Path: "/dev/tty", Err: syscall.ENXIO}
	cases := []struct {
		name string
		in   error
		want string
	}{
		{
			name: "no terminal collapses the framework's double prefix",
			in:   fmt.Errorf("bubbletea: error opening TTY: %w", fmt.Errorf("bubbletea: could not open TTY: %w", noTTY)),
			want: "no terminal: open /dev/tty: " + syscall.ENXIO.Error(),
		},
		{name: "other errors pass through", in: errors.New("program was killed"), want: "program was killed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runError(tc.in)
			if got.Error() != tc.want {
				t.Fatalf("runError = %q, want %q", got, tc.want)
			}
			if strings.Count(got.Error(), "bubbletea") != 0 {
				t.Fatalf("framework prefix leaked: %q", got)
			}
		})
	}
	if runError(nil) != nil {
		t.Fatal("runError(nil) must be nil")
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "ctrl+c during the preflight", err: startup.ErrAborted, want: exitInterrupted},
		{name: "wrapped abort", err: fmt.Errorf("preflight: %w", startup.ErrAborted), want: exitInterrupted},
		{name: "any other failure", err: errors.New(`context "x" not found in kubeconfig`), want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCode(tc.err); got != tc.want {
				t.Fatalf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestPrintRecap changes XDG_STATE_HOME, so it does not run in parallel:
// tui.New reads the saved port-forwards from there.
func TestPrintRecap(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	execed, _ := tui.New(nil, nil).Update(views.ExecRequestMsg{Namespace: "demo", Pod: "web-1"})
	recap := "kubectl commands from this k4s session:\n  kubectl exec -it -n demo web-1 -- sh\n"
	cases := []struct {
		name  string
		opts  cli.Options
		final tea.Model
		want  string
	}{
		{name: "the session's commands", final: execed, want: recap},
		{name: "--no-recap prints nothing", opts: cli.Options{NoRecap: true}, final: execed, want: ""},
		{name: "a session without commands", final: tui.New(nil, nil), want: ""},
		{name: "a program that never started", final: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printRecap(&out, tc.opts, tc.final)
			if got := out.String(); got != tc.want {
				t.Fatalf("printed %q, want %q", got, tc.want)
			}
		})
	}
}

// The command line and K4S_THEME must accept the same themes: --theme is
// handed to the theme setup in place of K4S_THEME.
func TestThemeFlagMatchesStyles(t *testing.T) {
	t.Parallel()

	for _, theme := range cli.Themes() {
		getenv := cli.Options{Theme: theme}.Getenv(func(string) string { return "" })
		if _, err := styles.ParseMode(getenv); err != nil {
			t.Errorf("--theme %s is not accepted by styles: %v", theme, err)
		}
	}
}
