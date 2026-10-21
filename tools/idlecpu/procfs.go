package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// userHZ is the unit of utime and stime in /proc/<pid>/stat. The kernel
// fixes USER_HZ at 100 for /proc on every architecture k4s ships for, and
// reading sysconf(_SC_CLK_TCK) would need cgo.
const userHZ = 100

// Fields 14 and 15 of /proc/<pid>/stat, counted from 1, as indexes into the
// fields after the comm field (which ends at field 2).
const (
	utimeField = 14 - 3
	stimeField = 15 - 3
)

// readProcSample reads the CPU time, resident set and peak resident set of
// pid from /proc.
func readProcSample(_ context.Context, pid int) (sample, error) {
	// G703: the path is /proc plus an int, nothing a caller can steer.
	dir := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(dir + "/stat") //nolint:gosec // see above
	if err != nil {
		return sample{}, fmt.Errorf("read %s/stat: %w", dir, err)
	}
	cpu, err := parseProcStat(stat)
	if err != nil {
		return sample{}, err
	}
	status, err := os.ReadFile(dir + "/status") //nolint:gosec // see above
	if err != nil {
		return sample{}, fmt.Errorf("read %s/status: %w", dir, err)
	}
	rss, err := parseStatusRSS(status)
	if err != nil {
		return sample{}, err
	}
	hwm, err := parseStatusHWM(status)
	if err != nil {
		return sample{}, err
	}
	return sample{cpu: cpu, rss: rss, hwm: hwm}, nil
}

// parseProcStat returns utime+stime from a /proc/<pid>/stat line.
func parseProcStat(data []byte) (time.Duration, error) {
	s := string(data)
	// comm may hold spaces and parentheses; only the last ')' ends it.
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return 0, errors.New("parse stat: no comm field")
	}
	fields := strings.Fields(s[end+1:])
	if len(fields) <= stimeField {
		return 0, fmt.Errorf("parse stat: %d fields after comm, want at least %d", len(fields), stimeField+1)
	}
	utime, err := parseTicks(fields[utimeField])
	if err != nil {
		return 0, fmt.Errorf("parse stat utime: %w", err)
	}
	stime, err := parseTicks(fields[stimeField])
	if err != nil {
		return 0, fmt.Errorf("parse stat stime: %w", err)
	}
	return time.Duration(utime+stime) * (time.Second / userHZ), nil
}

func parseTicks(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("negative tick count %d", n)
	}
	return n, nil
}

// parseStatusRSS returns VmRSS, in bytes, from /proc/<pid>/status.
func parseStatusRSS(data []byte) (int64, error) {
	kb, ok, err := statusKB(data, "VmRSS")
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.New("parse status: no VmRSS line")
	}
	return kb * 1024, nil
}

// parseStatusHWM returns VmHWM, the peak resident set the kernel has seen
// for the process, in bytes. A status without the line gives 0: the peak of
// the samples then stands alone.
func parseStatusHWM(data []byte) (int64, error) {
	kb, _, err := statusKB(data, "VmHWM")
	return kb * 1024, err
}

// statusKB reads a "<field>:\t<n> kB" line of /proc/<pid>/status; ok is
// false when the line is absent.
func statusKB(data []byte, field string) (kb int64, ok bool, err error) {
	for line := range strings.Lines(string(data)) {
		rest, found := strings.CutPrefix(line, field+":")
		if !found {
			continue
		}
		f := strings.Fields(rest)
		if len(f) != 2 || f[1] != "kB" {
			return 0, false, fmt.Errorf("parse status: %s is %q, want <n> kB", field, strings.TrimSpace(rest))
		}
		kb, err = strconv.ParseInt(f[0], 10, 64)
		if err != nil || kb < 0 {
			return 0, false, fmt.Errorf("parse %s %q: want a count of kB", field, f[0])
		}
		return kb, true, nil
	}
	return 0, false, nil
}
