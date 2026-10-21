package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// exitInterrupted is the shell convention for a run ended by SIGINT.
const exitInterrupted = 130

func main() {
	// Flags come first: --version and --help must not touch the terminal
	// or load a kubeconfig.
	opts, proceed, code := cli.Parse(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	if !proceed {
		os.Exit(code)
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "k4s: %v\n", err)
		os.Exit(exitCode(err))
	}
}

func run(opts cli.Options) error {
	// Before tea.NewProgram: auto mode queries the terminal background, and
	// that reply would race Bubble Tea's stdin reader.
	if err := styles.Init(opts.Getenv(os.Getenv)); err != nil {
		return fmt.Errorf("init theme: %w", err)
	}

	// Before the first clientset is built: client-go captures os.Stderr for
	// credential plugins at build time, and klog must already be off the
	// terminal once the TUI owns it.
	sink := logsink.Open()
	if err := sink.Install(); err != nil {
		return fmt.Errorf("install log sink: %w", err)
	}
	defer func() { _ = sink.Close() }()

	sess, err := openSession(opts)
	if err != nil {
		return err
	}
	logsink.HoldPluginStderr()

	var progOpts []tea.ProgramOption
	if profile, ok := styles.ColorProfile(); ok {
		progOpts = append(progOpts, tea.WithColorProfile(profile))
	}
	p := tea.NewProgram(tui.New(sess.Client, sess.LoadErr), progOpts...)
	_, err = p.Run()
	err = runError(err)

	// The TUI has released the terminal: show what was held back.
	_ = logsink.FlushPluginStderr(os.Stderr)
	if msg := sink.Summary(); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	return err
}

// openSession loads the kubeconfig and, unless --no-preflight, lets a
// credential plugin prompt on the plain terminal. Until the TUI puts the
// terminal in raw mode Ctrl+C is a signal, so it is caught to abort the
// preflight cleanly.
func openSession(opts cli.Options) (startup.Session, error) {
	sess, err := startup.Open(opts, startup.SystemEnv())
	if err != nil || opts.NoPreflight {
		return sess, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return startup.Preflight(ctx, sess, startup.PreflightConfig{Out: os.Stderr})
}

func exitCode(err error) int {
	if errors.Is(err, startup.ErrAborted) {
		return exitInterrupted
	}
	return 1
}

// runError shortens the framework's error when there is no terminal (ssh
// without -t, cron, a container without -t): it nests two "bubbletea:"
// prefixes around the open(/dev/tty) failure.
func runError(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Path == "/dev/tty" {
		return fmt.Errorf("no terminal: %w", pe)
	}
	return err
}
