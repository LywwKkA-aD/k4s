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
	// Best-effort kubeconfig load — failure is fine, the TUI will render a
	// "not connected" banner and let the user fix it interactively.
	client, _ := k8s.LoadFromKubeconfig("")

	p := tea.NewProgram(tui.New(client), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
