// Command idlecpu measures the CPU and memory k4s uses while it sits idle
// on a pty, for the E6 resource budget. make idle-cpu runs it against the
// local k3s; see idlecpu --help.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	exitOK    = 0
	exitGate  = 1 // a --max-cpu, --max-rss or --require-idle gate failed
	exitError = 2 // bad flags, or the run could not be measured
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "idlecpu: %v\n", err)
		return exitError
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := deps{sample: systemSampler()}
	if cfg.churn {
		d.pods = kubectlPods(cfg.kubectl, cfg.kubeconfig)
	}
	version := binaryVersion(ctx, cfg.bin)
	res, err := measure(ctx, cfg, d)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "idlecpu: %v\n", err)
		return exitError
	}
	h := host{goos: runtime.GOOS, goarch: runtime.GOARCH, cpus: runtime.NumCPU(), maxProcs: runtime.GOMAXPROCS(0)}
	rep := newReport(cfg, res, version, h)
	write := writeText
	if cfg.json {
		write = writeJSON
	}
	if err := write(stdout, rep); err != nil {
		_, _ = fmt.Fprintf(stderr, "idlecpu: %v\n", err)
		return exitError
	}
	if !rep.Pass {
		return exitGate
	}
	return exitOK
}

// binaryVersion is the first line of bin --version, so the report names
// the build it measured; empty when bin has no such flag.
func binaryVersion(ctx context.Context, bin string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output() //nolint:gosec // G204: the binary is the operator's own --bin
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line
}
