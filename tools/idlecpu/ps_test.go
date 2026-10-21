package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestParseCPUTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr string
	}{
		{name: "macOS minutes and hundredths", in: "0:00.05", want: 50 * time.Millisecond},
		{name: "macOS long-running", in: "26:21.68", want: 26*time.Minute + 21*time.Second + 680*time.Millisecond},
		{name: "macOS over an hour of minutes", in: "123:04.50", want: 123*time.Minute + 4500*time.Millisecond},
		{name: "procps hours", in: "01:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
		{name: "procps days", in: "2-03:04:05", want: 51*time.Hour + 4*time.Minute + 5*time.Second},
		{name: "nanosecond digits", in: "0:01.123456789", want: time.Second + 123456789},
		{name: "no colon", in: "5", wantErr: "want [[dd-]hh:]mm:ss"},
		{name: "too many colons", in: "1:2:3:4", wantErr: "want [[dd-]hh:]mm:ss"},
		{name: "empty part", in: "1::03", wantErr: "parse cpu time"},
		{name: "letters", in: "a:03", wantErr: "parse cpu time"},
		{name: "bad days", in: "x-01:02:03", wantErr: "parse cpu time"},
		{name: "bad fraction", in: "0:01.x", wantErr: "parse cpu time"},
		{name: "fraction too long", in: "0:01.1234567890", wantErr: "parse cpu time"},
		{name: "negative", in: "-1:00", wantErr: "parse cpu time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseCPUTime(tc.in)
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && got != tc.want {
				t.Fatalf("cpu = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParsePSOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    sample
		wantErr string
	}{
		{name: "macOS", in: "  0:00.05   4352\n", want: sample{cpu: 50 * time.Millisecond, rss: 4352 * 1024}},
		{name: "procps", in: "00:00:01 27340\n", want: sample{cpu: time.Second, rss: 27340 * 1024}},
		{name: "process gone", in: "", wantErr: "want time and rss"},
		{name: "rss missing", in: "0:00.05\n", wantErr: "want time and rss"},
		{name: "bad time", in: "soon 4352\n", wantErr: "parse cpu time"},
		{name: "bad rss", in: "0:00.05 big\n", wantErr: "parse rss"},
		{name: "negative rss", in: "0:00.05 -1\n", wantErr: "parse rss"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePSOutput([]byte(tc.in))
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && got != tc.want {
				t.Fatalf("sample = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSystemSamplerReadsItself(t *testing.T) {
	t.Parallel()

	got, err := systemSampler()(context.Background(), os.Getpid())
	if err != nil {
		t.Fatalf("sample self: %v", err)
	}
	if got.rss <= 0 {
		t.Fatalf("rss = %d, want > 0", got.rss)
	}
}

func TestSystemSamplerMissingProcess(t *testing.T) {
	t.Parallel()

	// pid_max is at most 2^22 on Linux and 99998 on macOS.
	const nobody = 1 << 30
	if _, err := systemSampler()(context.Background(), nobody); err == nil {
		t.Fatal("sampling a pid that does not exist returned no error")
	}
}
