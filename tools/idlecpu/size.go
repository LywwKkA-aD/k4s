package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// sizeUnits follows kubectl's quantities (M is 10^6, Mi is 2^20) and also
// takes the B-suffixed spellings, so "40MB" in the E6 budget means what it
// says.
var sizeUnits = map[string]float64{
	"B": 1,
	"k": 1e3, "K": 1e3, "KB": 1e3, "kB": 1e3, "Ki": 1 << 10, "KiB": 1 << 10,
	"M": 1e6, "MB": 1e6, "Mi": 1 << 20, "MiB": 1 << 20,
	"G": 1e9, "GB": 1e9, "Gi": 1 << 30, "GiB": 1 << 30,
}

// parseSize reads a byte count with a unit, such as 40MB or 38MiB.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	numEnd := strings.LastIndexAny(s, "0123456789.") + 1
	num, unit := s[:numEnd], strings.TrimSpace(s[numEnd:])
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("parse size %q: want a size such as 40MB or 38MiB", s)
	}
	if unit == "" {
		return 0, fmt.Errorf("parse size %q: want a unit, such as 40MB or 38MiB", s)
	}
	mult, ok := sizeUnits[unit]
	if !ok {
		return 0, fmt.Errorf("parse size %q: unknown unit %q (want B, kB, KiB, MB, MiB, GB or GiB)", s, unit)
	}
	bytes := math.Round(n * mult)
	if bytes < 1 || bytes > math.MaxInt64/2 {
		return 0, fmt.Errorf("parse size %q: want a size above 0", s)
	}
	return int64(bytes), nil
}

// parseWinsize reads a terminal size such as 120x40.
func parseWinsize(s string) (cols, rows uint16, err error) {
	c, r, ok := strings.Cut(strings.ToLower(s), "x")
	const limit = 1000
	nc, errC := strconv.Atoi(c)
	nr, errR := strconv.Atoi(r)
	if !ok || errC != nil || errR != nil || nc < 1 || nr < 1 || nc > limit || nr > limit {
		return 0, 0, fmt.Errorf("parse size %q: want COLSxROWS, each 1 to %d", s, limit)
	}
	return uint16(nc), uint16(nr), nil
}

// formatBytes prints n in MiB with MB alongside, since budgets are quoted
// in both.
func formatBytes(n int64) string {
	return fmt.Sprintf("%.1f MiB (%.1f MB)", float64(n)/(1<<20), float64(n)/1e6)
}
