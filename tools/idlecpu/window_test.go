package main

import (
	"math"
	"testing"
	"time"
)

func TestCPUPercent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cpu  time.Duration
		wall time.Duration
		want float64
	}{
		{name: "idle", cpu: 0, wall: 40 * time.Second, want: 0},
		{name: "e6 limit", cpu: 800 * time.Millisecond, wall: 40 * time.Second, want: 2},
		{name: "1.1.5 baseline", cpu: 500 * time.Millisecond, wall: 40 * time.Second, want: 1.25},
		{name: "one busy core", cpu: 40 * time.Second, wall: 40 * time.Second, want: 100},
		{name: "two busy cores", cpu: 80 * time.Second, wall: 40 * time.Second, want: 200},
		{name: "no wall time", cpu: time.Second, wall: 0, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cpuPercent(tc.cpu, tc.wall); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("cpuPercent(%v, %v) = %v, want %v", tc.cpu, tc.wall, got, tc.want)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	const mib = 1 << 20

	cases := []struct {
		name    string
		in      []sample
		want    windowStats
		wantErr string
	}{
		{
			name: "peak in the middle",
			in: []sample{
				{at: at(0), cpu: 2 * time.Second, rss: 25 * mib},
				{at: at(20), cpu: 2100 * time.Millisecond, rss: 29 * mib},
				{at: at(40), cpu: 2400 * time.Millisecond, rss: 27 * mib},
			},
			want: windowStats{
				wall: 40 * time.Second, cpu: 400 * time.Millisecond, cpuPct: 1,
				peakRSS: 29 * mib, finalRSS: 27 * mib, samples: 3,
			},
		},
		{
			name: "two samples",
			in:   []sample{{at: at(0), rss: mib}, {at: at(10), cpu: 100 * time.Millisecond, rss: 2 * mib}},
			want: windowStats{
				wall: 10 * time.Second, cpu: 100 * time.Millisecond, cpuPct: 1,
				peakRSS: 2 * mib, finalRSS: 2 * mib, samples: 2,
			},
		},
		{name: "one sample", in: []sample{{at: at(0)}}, wantErr: "want at least 2 samples"},
		{name: "none", wantErr: "want at least 2 samples"},
		{name: "no time passed", in: []sample{{at: at(1)}, {at: at(1)}}, wantErr: "no time passed"},
		{
			name:    "cpu time going back means another process",
			in:      []sample{{at: at(0), cpu: time.Second}, {at: at(1), cpu: 0}},
			wantErr: "went back",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := summarize(tc.in)
			checkErr(t, err, tc.wantErr)
			if tc.wantErr != "" {
				return
			}
			if math.Abs(got.cpuPct-tc.want.cpuPct) > 1e-9 {
				t.Fatalf("cpuPct = %v, want %v", got.cpuPct, tc.want.cpuPct)
			}
			got.cpuPct = tc.want.cpuPct
			if got != tc.want {
				t.Fatalf("stats = %+v, want %+v", got, tc.want)
			}
		})
	}
}
