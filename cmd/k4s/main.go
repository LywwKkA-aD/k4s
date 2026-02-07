package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/adapter/config"
	"github.com/LywwKkA-aD/k4s/internal/adapter/tui"
	"github.com/LywwKkA-aD/k4s/internal/logger"
)

// Version information - set via ldflags during build
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	// Parse command line flags
	versionFlag := flag.Bool("version", false, "Print version information")
	flag.BoolVar(versionFlag, "v", false, "Print version information (shorthand)")
	flag.Parse()

	if *versionFlag {
		printVersion()
		return
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("k4s - Kubernetes TUI for K3s\n")
	fmt.Printf("  Version:    %s\n", Version)
	fmt.Printf("  Commit:     %s\n", Commit)
	fmt.Printf("  Built:      %s\n", BuildDate)
	fmt.Printf("  Go version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func run() error {
	// Initialize logger
	if err := logger.Init(logger.LevelDebug); err != nil {
		// Non-fatal: continue without logging
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logger: %v\n", err)
	}
	defer logger.Close()

	logger.Info("Loading configuration")

	// Load configuration
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		logger.Error("Failed to load config", "err", err)
		return fmt.Errorf("load config: %w", err)
	}

	logger.Info("Found kubeconfigs", "count", len(cfg.KubeConfigs))

	// Create and run TUI
	app := tui.NewApp(cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())

	logger.Info("Starting TUI")

	if _, err := p.Run(); err != nil {
		logger.Error("TUI error", "err", err)
		return fmt.Errorf("run k4s: %w", err)
	}

	logger.Info("k4s exited normally")
	return nil
}
