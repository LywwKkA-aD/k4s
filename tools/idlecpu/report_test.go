package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func idleWindow() windowStats {
	return windowStats{
		wall: 40 * time.Second, cpu: 250 * time.Millisecond, cpuPct: 0.625,
		peakRSS: 27340 * 1024, finalRSS: 27000 * 1024, samples: 41,
	}
}

func TestEvaluateGates(t *testing.T) {
	t.Parallel()

	idle := podCheck{enabled: true, checked: true}
	busy := podCheck{enabled: true, checked: true, churn: churn{added: 1, updated: 2}}
	unknown := podCheck{enabled: true, err: "kubectl get pods: not found"}

	cases := []struct {
		name     string
		gates    gates
		pods     podCheck
		cpuPct   float64
		wantPass []bool
	}{
		{name: "no gates", gates: gates{}, pods: busy, cpuPct: 50, wantPass: nil},
		{name: "e6 pass", gates: gates{maxCPU: 2, maxRSS: 40_000_000}, pods: idle, cpuPct: 0.625, wantPass: []bool{true, true}},
		{name: "cpu over", gates: gates{maxCPU: 0.5}, pods: idle, cpuPct: 0.625, wantPass: []bool{false}},
		{name: "cpu equal after rounding", gates: gates{maxCPU: 2}, pods: idle, cpuPct: 2.004, wantPass: []bool{true}},
		{name: "rss over", gates: gates{maxRSS: 20 << 20}, pods: idle, cpuPct: 1, wantPass: []bool{false}},
		{name: "idle required and idle", gates: gates{requireIdle: true}, pods: idle, wantPass: []bool{true}},
		{name: "idle required, pods changed", gates: gates{requireIdle: true}, pods: busy, wantPass: []bool{false}},
		{name: "idle required, unknown", gates: gates{requireIdle: true}, pods: unknown, wantPass: []bool{false}},
		{name: "idle required, check off", gates: gates{requireIdle: true}, pods: podCheck{}, wantPass: []bool{false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := idleWindow()
			w.cpuPct = tc.cpuPct
			got := evaluate(tc.gates, w, tc.pods)
			var pass []bool
			for _, g := range got {
				pass = append(pass, g.Pass)
			}
			if !reflect.DeepEqual(pass, tc.wantPass) {
				t.Fatalf("gate results = %+v, want pass %v", got, tc.wantPass)
			}
		})
	}
}

func testReport(pods podCheck, g gates) report {
	cfg := config{
		bin: "bin/k4s", cols: 120, rows: 40, warmup: 3 * time.Second, duration: 40 * time.Second, gates: g,
	}
	res := result{
		window:      idleWindow(),
		output:      outputStats{bytes: 18_841, reads: 160},
		outputTotal: outputStats{bytes: 61_440, reads: 300},
		pods:        pods,
		quit:        "exit 0 on q",
	}
	return newReport(cfg, res, "k4s v1.2.0 (go1.26.10 linux/arm64)", host{goos: "linux", goarch: "arm64", cpus: 4, maxProcs: 1})
}

func TestWriteTextIdle(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	r := testReport(podCheck{enabled: true, checked: true}, gates{maxCPU: 2, maxRSS: 40_000_000})
	if err := writeText(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"k4s v1.2.0 (go1.26.10 linux/arm64)", "linux/arm64, 4 CPUs, GOMAXPROCS 1, pty 120x40",
		"window   40.00 s after a 3s warm-up, 41 samples",
		"cpu      0.62% of one core (0.250 s of cpu time)",
		"rss      peak 26.7 MiB (28.0 MB), final 26.4 MiB (27.6 MB)",
		"output   18.4 KiB in 160 reads (4.0 reads/s); 60.0 KiB since start",
		"cluster  idle: no pod was added, removed or changed",
		"gate     cpu 0.62% <= 2.00%: pass",
		"gate     rss 26.7 MiB (28.0 MB) <= 38.1 MiB (40.0 MB): pass",
		"quit     exit 0 on q",
		"result   PASS",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}

func TestWriteTextNotIdle(t *testing.T) {
	t.Parallel()

	busy := podCheck{
		enabled: true, checked: true,
		churn: churn{added: 2, removed: 1, updated: 2, byNamespace: []nsCount{{"k4s-demo", 4}, {"k4s-broken", 1}}},
	}
	cases := []struct {
		name  string
		pods  podCheck
		gates gates
		want  []string
	}{
		{
			name: "pods changed",
			pods: busy,
			want: []string{
				"cluster  NOT IDLE: 5 pod changes during the window (2 added, 1 removed, 2 updated): k4s-demo 4, k4s-broken 1",
				"result   PASS",
			},
		},
		{
			name:  "pods changed with --require-idle",
			pods:  busy,
			gates: gates{requireIdle: true},
			want:  []string{"gate     idle 5 pod changes <= 0: FAIL", "result   FAIL"},
		},
		{
			name: "kubectl failed",
			pods: podCheck{enabled: true, err: "before the window: kubectl get pods: exit status 1"},
			want: []string{"cluster  unknown: before the window: kubectl get pods: exit status 1"},
		},
		{
			name: "check off",
			pods: podCheck{},
			want: []string{"cluster  not checked (--churn=false)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := writeText(&out, testReport(tc.pods, tc.gates)); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("report lacks %q:\n%s", want, out.String())
				}
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	busy := podCheck{
		enabled: true, checked: true,
		churn: churn{updated: 3, byNamespace: []nsCount{{"k4s-demo", 3}}},
	}
	var out bytes.Buffer
	if err := writeJSON(&out, testReport(busy, gates{maxCPU: 2})); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	checks := map[string]any{
		"binary": "bin/k4s", "os": "linux", "arch": "arm64", "cpus": 4.0, "size": "120x40",
		"warmup_seconds": 3.0, "window_seconds": 40.0, "samples": 41.0,
		"cpu_seconds": 0.25, "cpu_percent": 0.625, "peak_rss_bytes": float64(27340 * 1024),
		"output_bytes": 18841.0, "output_reads": 160.0, "output_total_bytes": 61440.0,
		"gomaxprocs": 1.0, "quit": "exit 0 on q", "pass": true,
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	pods, ok := got["pods"].(map[string]any)
	if !ok || pods["changed"] != 3.0 || pods["checked"] != true {
		t.Errorf("pods = %v, want 3 changes", got["pods"])
	}
	if ns, ok := pods["by_namespace"].(map[string]any); !ok || ns["k4s-demo"] != 3.0 {
		t.Errorf("by_namespace = %v", pods["by_namespace"])
	}
}

func TestWriteJSONWithoutPodCheck(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := writeJSON(&out, testReport(podCheck{}, gates{})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `"pods"`) {
		t.Fatalf("pods present although the check was off:\n%s", out.String())
	}
}
