package main

import (
	"strings"
	"testing"
	"time"
)

// statLine is a /proc/<pid>/stat line with utime 37 and stime 12 ticks.
const statLine = "4242 (k4s) S 1 4242 4242 34816 4242 4194304 1520 0 0 0 37 12 0 0 20 0 9 0 123456 1300000000 6800 18446744073709551615\n"

const statusText = `Name:	k4s
Umask:	0022
State:	S (sleeping)
Pid:	4242
VmPeak:	 1312000 kB
VmHWM:	   29012 kB
VmRSS:	   27340 kB
RssAnon:	   12000 kB
Threads:	9
`

func TestParseProcStat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr string
	}{
		{name: "utime plus stime", in: statLine, want: 490 * time.Millisecond},
		{
			name: "comm with spaces and parentheses",
			in:   "7 (k4s (dev) x) R 1 7 7 0 -1 4194560 10 0 0 0 250 50 0 0 20 0 1 0 99 1000 10 0\n",
			want: 3 * time.Second,
		},
		{name: "zero ticks", in: "1 (a) S 0 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1 1 1 0", want: 0},
		{name: "no comm", in: "4242 k4s S 1", wantErr: "no comm field"},
		{name: "too few fields", in: "4242 (k4s) S 1 4242", wantErr: "want at least"},
		{name: "utime not a number", in: strings.Replace(statLine, " 37 ", " x ", 1), wantErr: "utime"},
		{name: "stime not a number", in: strings.Replace(statLine, " 37 12 ", " 37 y ", 1), wantErr: "stime"},
		{name: "negative ticks", in: strings.Replace(statLine, " 37 ", " -37 ", 1), wantErr: "utime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseProcStat([]byte(tc.in))
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && got != tc.want {
				t.Fatalf("cpu = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseStatusRSS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr string
	}{
		{name: "VmRSS in kB", in: statusText, want: 27340 * 1024},
		{name: "last line without newline", in: "Name:\tk4s\nVmRSS:\t12 kB", want: 12 * 1024},
		{name: "no VmRSS for a zombie", in: "Name:\tk4s\nState:\tZ (zombie)\n", wantErr: "no VmRSS line"},
		{name: "unit is not kB", in: "VmRSS:\t12 MB\n", wantErr: "want <n> kB"},
		{name: "value missing", in: "VmRSS:\n", wantErr: "want <n> kB"},
		{name: "value not a number", in: "VmRSS:\tlots kB\n", wantErr: "parse VmRSS"},
		{name: "negative value", in: "VmRSS:\t-5 kB\n", wantErr: "parse VmRSS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseStatusRSS([]byte(tc.in))
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && got != tc.want {
				t.Fatalf("rss = %d, want %d", got, tc.want)
			}
		})
	}
}

// checkErr fails t unless err is nil when want is empty, or contains want.
func checkErr(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}
