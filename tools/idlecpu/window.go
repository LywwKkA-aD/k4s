package main

import (
	"errors"
	"fmt"
	"time"
)

// sample is one reading of the child taken at a point in time.
type sample struct {
	at  time.Time
	cpu time.Duration // utime+stime since the process started
	rss int64         // resident set, bytes
	hwm int64         // peak resident set since start, bytes; 0 when unknown (VmHWM, Linux)
}

// windowStats sums up the samples taken over the measurement window.
type windowStats struct {
	wall     time.Duration
	cpu      time.Duration
	cpuPct   float64 // of one core
	peakRSS  int64
	finalRSS int64
	samples  int
}

// summarize turns the samples of one window, oldest first, into its CPU
// share and RSS figures. The peak is the larger of the sampled resident sets
// and the kernel's high-water mark, so a spike between two samples counts.
func summarize(samples []sample) (windowStats, error) {
	if len(samples) < 2 {
		return windowStats{}, fmt.Errorf("summarize window: %d samples, want at least 2 samples", len(samples))
	}
	first, last := samples[0], samples[len(samples)-1]
	wall := last.at.Sub(first.at)
	if wall <= 0 {
		return windowStats{}, errors.New("summarize window: no time passed between the first and the last sample")
	}
	cpu := last.cpu - first.cpu
	if cpu < 0 {
		return windowStats{}, fmt.Errorf("summarize window: cpu time went back from %v to %v", first.cpu, last.cpu)
	}
	peak := int64(0)
	for _, s := range samples {
		peak = max(peak, s.rss, s.hwm)
	}
	return windowStats{
		wall:     wall,
		cpu:      cpu,
		cpuPct:   cpuPercent(cpu, wall),
		peakRSS:  peak,
		finalRSS: last.rss,
		samples:  len(samples),
	}, nil
}

// cpuPercent is cpu as a share of one core over wall.
func cpuPercent(cpu, wall time.Duration) float64 {
	if wall <= 0 {
		return 0
	}
	return 100 * cpu.Seconds() / wall.Seconds()
}
