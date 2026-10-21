package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// warmUpTimeout bounds the pre-TUI request that lets a credential plugin
// prompt for a login (SSO device codes, MFA) on the plain terminal.
const warmUpTimeout = 30 * time.Second

func main() {
	// Flags come first: --version and --help must not touch the terminal
	// or load a kubeconfig.
	if proceed, code := cli.Parse(os.Args[1:], os.Stdout, os.Stderr); !proceed {
		os.Exit(code)
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "k4s: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Before tea.NewProgram: auto mode queries the terminal background, and
	// that reply would race Bubble Tea's stdin reader.
	if err := styles.Init(os.Getenv); err != nil {
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

	// A load failure is not fatal: the dashboard shows the reason and the
	// user can still pick a working context with :ctx.
	client, loadErr := k8s.LoadFromKubeconfig("")
	if client.NeedsLogin() {
		warmUp(client)
	}
	logsink.HoldPluginStderr()

	var opts []tea.ProgramOption
	if profile, ok := styles.ColorProfile(); ok {
		opts = append(opts, tea.WithColorProfile(profile))
	}
	p := tea.NewProgram(tui.New(client, loadErr), opts...)
	_, err := p.Run()
	err = runError(err)

	// The TUI has released the terminal: show what was held back.
	_ = logsink.FlushPluginStderr(os.Stderr)
	if msg := sink.Summary(); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	return err
}

// warmUp runs the credential plugin once while the terminal is still plain,
// so its prompt is visible and its token cached before the TUI starts. A
// failure is left to the TUI, which shows request errors in place.
func warmUp(client *k8s.Client) {
	fmt.Fprintf(os.Stderr, "k4s: connecting to %s…\n", client.Context)
	ctx, cancel := context.WithTimeout(context.Background(), warmUpTimeout)
	defer cancel()
	_ = client.WarmUp(ctx)
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
