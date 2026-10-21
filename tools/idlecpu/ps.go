package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// sampler reads the cumulative CPU time and resident set of a process.
type sampler func(ctx context.Context, pid int) (sample, error)

// systemSampler reads /proc on Linux, where ps rounds CPU time to whole
// seconds, and ps elsewhere (macOS prints hundredths).
func systemSampler() sampler {
	if runtime.GOOS == "linux" {
		return readProcSample
	}
	return readPSSample
}

// readPSSample runs ps for pid's CPU time and resident set.
func readPSSample(ctx context.Context, pid int) (sample, error) {
	out, err := exec.CommandContext(ctx, "ps", "-o", "time=,rss=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // G204: a fixed command; pid is an int

	if err != nil {
		return sample{}, fmt.Errorf("run ps -p %d: %w", pid, err)
	}
	return parsePSOutput(out)
}

// parsePSOutput reads one "time rss" line of ps -o time=,rss= output; rss
// is in KiB.
func parsePSOutput(out []byte) (sample, error) {
	f := strings.Fields(string(out))
	if len(f) != 2 {
		return sample{}, fmt.Errorf("parse ps output %q: want time and rss", out)
	}
	cpu, err := parseCPUTime(f[0])
	if err != nil {
		return sample{}, err
	}
	kib, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil || kib < 0 {
		return sample{}, fmt.Errorf("parse rss %q: want a count of KiB", f[1])
	}
	return sample{cpu: cpu, rss: kib * 1024}, nil
}

// parseCPUTime reads ps's time column, [[dd-]hh:]mm:ss[.fraction]: macOS
// prints minutes (unbounded) and hundredths, procps prints hh:mm:ss.
func parseCPUTime(s string) (time.Duration, error) {
	rest := s
	var days int64
	if d, r, ok := strings.Cut(s, "-"); ok {
		n, err := parseUint(d)
		if err != nil {
			return 0, fmt.Errorf("parse cpu time %q: days: %w", s, err)
		}
		days, rest = n, r
	}
	parts := strings.Split(rest, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("parse cpu time %q: want [[dd-]hh:]mm:ss", s)
	}
	whole, frac, _ := strings.Cut(parts[len(parts)-1], ".")
	parts[len(parts)-1] = whole

	total := time.Duration(days) * 24 * time.Hour
	unit := time.Second
	for i := len(parts) - 1; i >= 0; i-- {
		n, err := parseUint(parts[i])
		if err != nil {
			return 0, fmt.Errorf("parse cpu time %q: %w", s, err)
		}
		total += time.Duration(n) * unit
		unit *= 60
	}
	ns, err := parseFraction(frac)
	if err != nil {
		return 0, fmt.Errorf("parse cpu time %q: %w", s, err)
	}
	return total + ns, nil
}

func parseUint(s string) (int64, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, fmt.Errorf("%q is not a whole number", s)
	}
	return strconv.ParseInt(s, 10, 64)
}

// parseFraction reads the digits after a decimal point as nanoseconds.
func parseFraction(digits string) (time.Duration, error) {
	if digits == "" {
		return 0, nil
	}
	if len(digits) > 9 {
		return 0, fmt.Errorf("fraction %q is finer than a nanosecond", digits)
	}
	n, err := parseUint(digits + strings.Repeat("0", 9-len(digits)))
	if err != nil {
		return 0, err
	}
	return time.Duration(n), nil
}
