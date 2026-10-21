package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// config is one harness run, from the command line.
type config struct {
	bin         string
	args        []string // for k4s, after --
	kubeconfig  string
	duration    time.Duration
	warmup      time.Duration
	interval    time.Duration
	cols, rows  uint16
	term        string
	env         []string // KEY=VALUE for k4s, applied last
	gates       gates
	churn       bool
	kubectl     string
	quitKeys    string
	quitTimeout time.Duration
	json        bool
}

// gates are the limits that fail a run; zero values are off.
type gates struct {
	maxCPU      float64 // percent of one core
	maxRSS      int64   // bytes, against the peak sample
	requireIdle bool
}

const usageHead = `Usage of idlecpu:
  idlecpu [flags] [-- k4s flags]

idlecpu runs k4s under a pseudo-terminal, waits out a warm-up, then
measures its CPU time (utime+stime) and resident set over a window in
which nobody types.

Pods are listed with kubectl before and after the window. A CronJob or a
crash loop makes the cluster non-idle, and the figures then include k4s
reacting to it; the report says so, and --require-idle fails the run.

Exit codes: 0 pass, 1 a gate failed, 2 the run itself failed.

Flags:
`

// parseConfig reads the flags; anything after -- goes to k4s.
func parseConfig(args []string, stderr io.Writer) (config, error) {
	cfg := config{}
	fs := flag.NewFlagSet("idlecpu", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), usageHead)
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.bin, "bin", "bin/k4s", "k4s `binary` to run")
	fs.StringVar(&cfg.kubeconfig, "kubeconfig", "", "kubeconfig `file` for k4s and kubectl (default: $KUBECONFIG)")
	fs.DurationVar(&cfg.duration, "duration", 40*time.Second, "measurement window")
	fs.DurationVar(&cfg.warmup, "warmup", 3*time.Second, "time from start to the window")
	fs.DurationVar(&cfg.interval, "interval", time.Second, "sampling interval inside the window (sets the RSS peak resolution)")
	size := fs.String("size", "120x40", "pty size, `COLSxROWS`")
	fs.StringVar(&cfg.term, "term", "xterm-256color", "TERM for k4s; COLORTERM, TERM_PROGRAM, TMUX, NO_COLOR and K4S_THEME are not inherited")
	fs.Func("env", "extra `KEY=VALUE` for k4s, repeatable (e.g. K4S_THEME=mono)", func(s string) error {
		if k, _, ok := strings.Cut(s, "="); !ok || k == "" {
			return errors.New("want KEY=VALUE")
		}
		cfg.env = append(cfg.env, s)
		return nil
	})
	fs.Float64Var(&cfg.gates.maxCPU, "max-cpu", 0, "fail above this CPU `percent` of one core (0: no gate)")
	rss := fs.String("max-rss", "", "fail when the peak RSS is above this `size`, e.g. 40MB or 38MiB")
	fs.BoolVar(&cfg.gates.requireIdle, "require-idle", false, "fail unless the pod check ran and saw no pod change")
	fs.BoolVar(&cfg.churn, "churn", true, "list pods with kubectl before and after the window")
	fs.StringVar(&cfg.kubectl, "kubectl", "kubectl", "kubectl `binary` for the pod check")
	quit := fs.String("quit-keys", "q", "`keys` sent to quit; Go escapes such as \\x03 work")
	fs.DurationVar(&cfg.quitTimeout, "quit-timeout", 5*time.Second, "wait after the quit keys, and again after SIGTERM, before escalating")
	fs.BoolVar(&cfg.json, "json", false, "print the report as JSON")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if rest := fs.Args(); len(rest) > 0 {
		cfg.args = slices.Clone(rest)
	}
	var err error
	if cfg.cols, cfg.rows, err = parseWinsize(*size); err != nil {
		return config{}, err
	}
	if *rss != "" {
		if cfg.gates.maxRSS, err = parseSize(*rss); err != nil {
			return config{}, err
		}
	}
	if cfg.quitKeys, err = unquoteKeys(*quit); err != nil {
		return config{}, err
	}
	return cfg, validate(cfg)
}

func unquoteKeys(s string) (string, error) {
	keys, err := strconv.Unquote(`"` + strings.ReplaceAll(s, `"`, `\"`) + `"`)
	if err != nil {
		return "", fmt.Errorf("parse --quit-keys %q: %w", s, err)
	}
	return keys, nil
}

func validate(cfg config) error {
	if err := checkExecutable(cfg.bin); err != nil {
		return err
	}
	switch {
	case cfg.duration <= 0:
		return errors.New("--duration must be above 0")
	case cfg.warmup < 0:
		return errors.New("--warmup must not be negative")
	case cfg.interval <= 0 || cfg.interval > cfg.duration:
		return errors.New("--interval must be above 0 and at most --duration")
	case cfg.quitTimeout <= 0:
		return errors.New("--quit-timeout must be above 0")
	case cfg.gates.maxCPU < 0:
		return errors.New("--max-cpu must not be negative")
	case cfg.term == "":
		return errors.New("--term must not be empty")
	case cfg.quitKeys == "":
		return errors.New("--quit-keys must not be empty")
	case cfg.churn && cfg.kubectl == "":
		return errors.New("--kubectl must not be empty while --churn is on")
	}
	if cfg.kubeconfig == "" {
		return nil
	}
	if _, err := os.Stat(cfg.kubeconfig); err != nil {
		return fmt.Errorf("stat kubeconfig: %w", err)
	}
	return nil
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat binary (run make build first?): %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("binary %s is not an executable file", path)
	}
	return nil
}

// terminalEnv names the variables that identify the terminal of the shell
// running the harness. k4s picks its colour profile from them, so they are
// dropped to make runs compare across machines; --env puts any back.
var terminalEnv = []string{
	"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TMUX", "TMUX_PANE", "NO_COLOR", "K4S_THEME",
}

// childEnv is k4s's environment: parent without terminalEnv, then TERM,
// KUBECONFIG and extra. exec keeps the last of duplicate keys.
func childEnv(parent []string, term, kubeconfig string, extra []string) []string {
	drop := terminalEnv
	if kubeconfig != "" {
		drop = append(slices.Clone(terminalEnv), "KUBECONFIG")
	}
	out := make([]string, 0, len(parent)+2+len(extra))
	for _, kv := range parent {
		if k, _, _ := strings.Cut(kv, "="); !slices.Contains(drop, k) {
			out = append(out, kv)
		}
	}
	out = append(out, "TERM="+term)
	if kubeconfig != "" {
		out = append(out, "KUBECONFIG="+kubeconfig)
	}
	return append(out, extra...)
}
