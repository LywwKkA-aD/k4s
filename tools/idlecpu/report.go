package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// report is one run's figures, printed as text or, with --json, as JSON
// for the PR record.
type report struct {
	Binary        string       `json:"binary"`
	Version       string       `json:"version,omitempty"`
	OS            string       `json:"os"`
	Arch          string       `json:"arch"`
	CPUs          int          `json:"cpus"`
	GOMAXPROCS    int          `json:"gomaxprocs"`
	Size          string       `json:"size"`
	WarmupSeconds float64      `json:"warmup_seconds"`
	WindowSeconds float64      `json:"window_seconds"`
	Samples       int          `json:"samples"`
	CPUSeconds    float64      `json:"cpu_seconds"`
	CPUPercent    float64      `json:"cpu_percent"`
	PeakRSSBytes  int64        `json:"peak_rss_bytes"`
	FinalRSSBytes int64        `json:"final_rss_bytes"`
	OutputBytes   int64        `json:"output_bytes"`
	OutputReads   int64        `json:"output_reads"`
	OutputTotal   int64        `json:"output_total_bytes"`
	Pods          *podReport   `json:"pods,omitempty"`
	Quit          string       `json:"quit"`
	Gates         []gateResult `json:"gates"`
	Pass          bool         `json:"pass"`
}

// podReport is the pod check; nil in a report when --churn=false.
type podReport struct {
	Checked     bool           `json:"checked"`
	Error       string         `json:"error,omitempty"`
	Added       int            `json:"added"`
	Removed     int            `json:"removed"`
	Updated     int            `json:"updated"`
	Changed     int            `json:"changed"`
	ByNamespace map[string]int `json:"by_namespace,omitempty"`
}

type gateResult struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Limit string `json:"limit"`
	Pass  bool   `json:"pass"`
}

// host describes the machine the harness runs on. NumCPU ignores a
// container's CPU quota; GOMAXPROCS follows it, so a 1-CPU container shows.
type host struct {
	goos, goarch   string
	cpus, maxProcs int
}

func newReport(cfg config, res result, version string, h host) report {
	gs := evaluate(cfg.gates, res.window, res.pods)
	pass := true
	for _, g := range gs {
		pass = pass && g.Pass
	}
	return report{
		Binary:        cfg.bin,
		Version:       version,
		OS:            h.goos,
		Arch:          h.goarch,
		CPUs:          h.cpus,
		GOMAXPROCS:    h.maxProcs,
		Size:          fmt.Sprintf("%dx%d", cfg.cols, cfg.rows),
		WarmupSeconds: cfg.warmup.Seconds(),
		WindowSeconds: res.window.wall.Seconds(),
		Samples:       res.window.samples,
		CPUSeconds:    res.window.cpu.Seconds(),
		CPUPercent:    res.window.cpuPct,
		PeakRSSBytes:  res.window.peakRSS,
		FinalRSSBytes: res.window.finalRSS,
		OutputBytes:   res.output.bytes,
		OutputReads:   res.output.reads,
		OutputTotal:   res.outputTotal.bytes,
		Pods:          newPodReport(res.pods),
		Quit:          res.quit,
		Gates:         gs,
		Pass:          pass,
	}
}

func newPodReport(p podCheck) *podReport {
	if !p.enabled {
		return nil
	}
	var byNS map[string]int
	if len(p.churn.byNamespace) > 0 {
		byNS = make(map[string]int, len(p.churn.byNamespace))
		for _, c := range p.churn.byNamespace {
			byNS[c.ns] = c.n
		}
	}
	return &podReport{
		Checked: p.checked, Error: p.err,
		Added: p.churn.added, Removed: p.churn.removed, Updated: p.churn.updated, Changed: p.churn.total(),
		ByNamespace: byNS,
	}
}

// evaluate checks the window against the gates that are on.
func evaluate(g gates, w windowStats, p podCheck) []gateResult {
	var out []gateResult
	if g.maxCPU > 0 {
		// Compare what the report prints, so 2.004% never fails a 2.00% gate.
		shown := strconv.FormatFloat(w.cpuPct, 'f', 2, 64)
		v, _ := strconv.ParseFloat(shown, 64)
		out = append(out, gateResult{
			Name: "cpu", Value: shown + "%", Limit: strconv.FormatFloat(g.maxCPU, 'f', 2, 64) + "%", Pass: v <= g.maxCPU,
		})
	}
	if g.maxRSS > 0 {
		out = append(out, gateResult{
			Name: "rss", Value: formatBytes(w.peakRSS), Limit: formatBytes(g.maxRSS), Pass: w.peakRSS <= g.maxRSS,
		})
	}
	if g.requireIdle {
		value := "unknown"
		switch {
		case !p.enabled:
			value = "not checked"
		case p.checked:
			value = fmt.Sprintf("%d pod changes", p.churn.total())
		}
		out = append(out, gateResult{Name: "idle", Value: value, Limit: "0", Pass: p.checked && p.churn.total() == 0})
	}
	return out
}

func writeJSON(w io.Writer, r report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func writeText(w io.Writer, r report) error {
	var b bytes.Buffer
	line := func(label, format string, args ...any) {
		_, _ = fmt.Fprintf(&b, "%-8s %s\n", label, fmt.Sprintf(format, args...))
	}
	binary := r.Binary
	if r.Version != "" {
		binary += ", " + r.Version
	}
	line("binary", "%s", binary)
	line("host", "%s/%s, %d CPUs, GOMAXPROCS %d, pty %s", r.OS, r.Arch, r.CPUs, r.GOMAXPROCS, r.Size)
	warmup := time.Duration(r.WarmupSeconds * float64(time.Second))
	line("window", "%.2f s after a %s warm-up, %d samples", r.WindowSeconds, warmup, r.Samples)
	line("cpu", "%.2f%% of one core (%.3f s of cpu time)", r.CPUPercent, r.CPUSeconds)
	line("rss", "peak %s, final %s", formatBytes(r.PeakRSSBytes), formatBytes(r.FinalRSSBytes))
	// Bytes since start show k4s did draw when an idle window has none.
	line("output", "%.1f KiB in %d reads (%.1f reads/s); %.1f KiB since start",
		float64(r.OutputBytes)/1024, r.OutputReads, perSecond(r.OutputReads, r.WindowSeconds), float64(r.OutputTotal)/1024)
	line("cluster", "%s", describePods(r.Pods))
	for _, g := range r.Gates {
		line("gate", "%s %s <= %s: %s", g.Name, g.Value, g.Limit, passWord(g.Pass, "pass"))
	}
	line("quit", "%s", r.Quit)
	line("result", "%s", passWord(r.Pass, "PASS"))
	if _, err := w.Write(b.Bytes()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func describePods(p *podReport) string {
	switch {
	case p == nil:
		return "not checked (--churn=false)"
	case !p.Checked:
		return "unknown: " + p.Error
	case p.Changed == 0:
		return "idle: no pod was added, removed or changed"
	}
	parts := make([]string, 0, len(p.ByNamespace))
	for _, c := range sortedCounts(p.ByNamespace) {
		parts = append(parts, fmt.Sprintf("%s %d", c.ns, c.n))
	}
	return fmt.Sprintf("NOT IDLE: %d pod changes during the window (%d added, %d removed, %d updated): %s\n"+
		"         the figures include k4s reacting to them; measure without per-minute CronJob seeds",
		p.Changed, p.Added, p.Removed, p.Updated, strings.Join(parts, ", "))
}

func perSecond(n int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(n) / seconds
}

func passWord(pass bool, word string) string {
	if pass {
		return word
	}
	return "FAIL"
}
