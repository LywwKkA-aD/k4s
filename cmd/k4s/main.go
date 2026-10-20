package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "k4s: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// A load failure is not fatal: the dashboard shows the reason and the
	// user can still pick a working context with :ctx.
	client, loadErr := k8s.LoadFromKubeconfig("")

	p := tea.NewProgram(tui.New(client, loadErr), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
