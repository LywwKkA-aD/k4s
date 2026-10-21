package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeTUIEnv makes the test binary act as a stand-in for k4s; its value
// picks the behaviour (see fakeTUI).
const fakeTUIEnv = "IDLECPU_FAKE_TUI"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeTUIEnv); mode != "" {
		os.Exit(fakeTUI(mode))
	}
	os.Exit(m.Run())
}

// fakeTUI asks for the device attributes once, then draws a frame every
// 20 ms. In "draw" mode it quits on a line ending in "q": the pty stays in
// canonical mode, so the harness sends "q\n", and the line starts with the
// harness's reply to the query. "ignore-quit" waits for SIGTERM,
// "ignore-term" for SIGKILL, and "exit-early" exits 3 at once.
func fakeTUI(mode string) int {
	if mode == "exit-early" {
		return 3
	}
	fmt.Print("\x1b[c")
	go func() {
		for {
			fmt.Print("\x1b[Hframe")
			time.Sleep(20 * time.Millisecond)
		}
	}()
	switch mode {
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		select {}
	case "ignore-quit":
		select {}
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if strings.HasSuffix(sc.Text(), "q") {
			return 0
		}
	}
	return 1
}

func fakeConfig(t *testing.T, mode string) config {
	t.Helper()
	return config{
		bin:         testExe(t),
		duration:    300 * time.Millisecond,
		warmup:      100 * time.Millisecond,
		interval:    50 * time.Millisecond,
		cols:        100,
		rows:        30,
		term:        "xterm-256color",
		env:         []string{fakeTUIEnv + "=" + mode},
		quitKeys:    "q\n",
		quitTimeout: 2 * time.Second,
	}
}

func TestMeasureFakeTUI(t *testing.T) {
	t.Parallel()

	snapshots := []podSet{{"k4s-demo/a": "1"}, {"k4s-demo/a": "2"}}
	lister := func(context.Context) (podSet, error) {
		next := snapshots[0]
		snapshots = snapshots[1:]
		return next, nil
	}
	res, err := measure(context.Background(), fakeConfig(t, "draw"), deps{sample: systemSampler(), pods: lister})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if res.window.samples < 2 || res.window.wall < 300*time.Millisecond {
		t.Errorf("window = %+v, want at least 2 samples over 300ms", res.window)
	}
	if res.window.peakRSS <= 0 || res.window.finalRSS <= 0 {
		t.Errorf("rss not sampled: %+v", res.window)
	}
	if res.output.bytes <= 0 || res.output.reads <= 0 {
		t.Errorf("no output counted in the window: %+v", res.output)
	}
	if res.outputTotal.bytes <= res.output.bytes {
		t.Errorf("output since start (%d bytes) does not include the warm-up beyond the window (%d bytes)",
			res.outputTotal.bytes, res.output.bytes)
	}
	if !res.pods.checked || res.pods.churn.total() != 1 {
		t.Errorf("pods = %+v, want one updated pod", res.pods)
	}
	if res.quit != "exit 0 on q" {
		t.Errorf("quit = %q, want exit 0 on q", res.quit)
	}
}

func TestMeasureStopFallbacks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode string
		want string
	}{
		{mode: "ignore-quit", want: "after SIGTERM"},
		{mode: "ignore-term", want: "after SIGKILL"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()
			cfg := fakeConfig(t, tc.mode)
			cfg.quitTimeout = 200 * time.Millisecond
			res, err := measure(context.Background(), cfg, deps{sample: systemSampler()})
			if err != nil {
				t.Fatalf("measure: %v", err)
			}
			if !strings.HasSuffix(res.quit, tc.want) {
				t.Fatalf("quit = %q, want a suffix %q", res.quit, tc.want)
			}
			if res.pods.enabled {
				t.Fatalf("pod check ran without a lister: %+v", res.pods)
			}
		})
	}
}

func TestMeasureErrors(t *testing.T) {
	t.Parallel()

	errSample := errors.New("no such process")
	failing := func(context.Context, int) (sample, error) { return sample{}, errSample }

	cases := []struct {
		name    string
		mode    string
		bin     string
		sampler sampler
		wantErr string
	}{
		{name: "child exits during the warm-up", mode: "exit-early", wantErr: "exited during the warm-up: exit status 3"},
		{name: "sampler fails", mode: "draw", sampler: failing, wantErr: "sample pid"},
		{name: "binary cannot start", mode: "draw", bin: filepath.Join(t.TempDir(), "k4s"), wantErr: "start "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := fakeConfig(t, tc.mode)
			if tc.bin != "" {
				cfg.bin = tc.bin
			}
			s := tc.sampler
			if s == nil {
				s = systemSampler()
			}
			_, err := measure(context.Background(), cfg, deps{sample: s})
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestMeasureCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cfg := fakeConfig(t, "draw")
	cfg.duration = time.Minute
	calls := 0
	s := func(ctx context.Context, pid int) (sample, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return systemSampler()(ctx, pid)
	}
	start := time.Now()
	_, err := measure(ctx, cfg, deps{sample: s})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("cancelled run took %v", time.Since(start))
	}
}
