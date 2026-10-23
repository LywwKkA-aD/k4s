package dash

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	day  = 24 * time.Hour
	year = 365 * day
)

// parseAge reads kubectl's HumanDuration format ("45s", "2m30s", "5d4h",
// "3y45d"), which server-side Tables use for AGE and the RESTARTS suffix.
// A cell past what a Duration holds is rejected, not wrapped negative.
func parseAge(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	var total time.Duration
	for s != "" {
		i := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
		if i <= 0 {
			return 0, false
		}
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, false
		}
		unit, ok := ageUnit(s[i])
		if !ok || int64(n) > math.MaxInt64/int64(unit) {
			return 0, false
		}
		part := time.Duration(n) * unit
		if total > math.MaxInt64-part {
			return 0, false
		}
		total += part
		s = s[i+1:]
	}
	return total, true
}

func ageUnit(b byte) (time.Duration, bool) {
	switch b {
	case 's':
		return time.Second, true
	case 'm':
		return time.Minute, true
	case 'h':
		return time.Hour, true
	case 'd':
		return day, true
	case 'y':
		return year, true
	default:
		return 0, false
	}
}

// parseRestarts reads a pods Table RESTARTS cell: "0" or "41 (2m ago)".
// hasAgo is false when the suffix is missing or unreadable.
func parseRestarts(s string) (count int, ago time.Duration, hasAgo, ok bool) {
	head, tail, _ := strings.Cut(s, " ")
	count, err := strconv.Atoi(head)
	if err != nil {
		return 0, 0, false, false
	}
	inner, found := strings.CutPrefix(tail, "(")
	if !found {
		return count, 0, false, true
	}
	inner, found = strings.CutSuffix(inner, " ago)")
	if !found {
		return count, 0, false, true
	}
	ago, hasAgo = parseAge(inner)
	return count, ago, hasAgo, true
}

// Coarsen rounds an elapsed time down to the unit a glance needs: zero under
// a minute, then whole minutes, hours and days. Panels store relative times
// coarsened, so a poll that only moves the clock hashes the same.
func Coarsen(d time.Duration) time.Duration {
	switch {
	case d < time.Minute:
		return 0
	case d < time.Hour:
		return d.Truncate(time.Minute)
	case d < day:
		return d.Truncate(time.Hour)
	default:
		return d.Truncate(day)
	}
}

// FormatAgo renders a coarsened duration as "<1m", "5m", "3h" or "12d".
func FormatAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < day:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	default:
		return strconv.Itoa(int(d/day)) + "d"
	}
}

// since is now-t coarsened, or zero for a zero t.
func since(now, t time.Time) time.Duration {
	if t.IsZero() {
		return 0
	}
	return Coarsen(now.Sub(t))
}
