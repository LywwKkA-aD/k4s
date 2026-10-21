package tuitest_test

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

type fixedView string

func (v fixedView) View() string { return string(v) }

func TestStripAndScreen(t *testing.T) {
	t.Parallel()

	coloured := "\x1b[1;38;5;205mk4s\x1b[0m  \x1b[2mctx: fake\x1b[0m"
	if got := tuitest.Strip(coloured); got != "k4s  ctx: fake" {
		t.Errorf("Strip = %q", got)
	}
	if got := tuitest.Screen(fixedView(coloured)); got != "k4s  ctx: fake" {
		t.Errorf("Screen = %q", got)
	}
}

func TestTrimLines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "trailing spaces per line", in: "a   \nb  ", want: "a\nb"},
		{name: "trailing blank lines", in: "a\n  \n\n", want: "a"},
		{name: "leading space kept", in: "  a", want: "  a"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tuitest.TrimLines(tc.in); got != tc.want {
				t.Errorf("TrimLines(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHeaderFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		screen string
		ns     string
		view   string
	}{
		{name: "full header", screen: " k4s · k3s v1.36.5 · ctx default (10.0.0.5) · ns ALL · dashboard   \n────", ns: "ALL", view: "dashboard"},
		{name: "no client", screen: " k4s · ns demo · pods\n────", ns: "demo", view: "pods"},
		{name: "a title with separators", screen: " k4s · ctx fake · ns demo · logs · web-1 · /needle\n", ns: "demo", view: "logs · web-1 · /needle"},
		{name: "no header", screen: "body only", ns: "", view: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := fixedView(tc.screen)
			if got := tuitest.HeaderNamespace(v); got != tc.ns {
				t.Errorf("HeaderNamespace = %q, want %q", got, tc.ns)
			}
			if got := tuitest.HeaderView(v); got != tc.view {
				t.Errorf("HeaderView = %q, want %q", got, tc.view)
			}
		})
	}
}

func TestGolden(t *testing.T) {
	t.Parallel()

	tuitest.Golden(t, "sample", "line one\nline two\n")

	cases := []struct {
		name string
		file string
		got  string
		want string
	}{
		{name: "mismatch names the line", file: "sample", got: "line one\nline 2\n", want: "line 2:"},
		{name: "missing file", file: "does-not-exist", got: "x", want: "-update"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fatals, errs := record(func(tb testing.TB) { tuitest.Golden(tb, tc.file, tc.got) }).failures()
			all := append(fatals, errs...)
			if !anyContains(all, tc.want) {
				t.Errorf("failures %q, want one containing %q", all, tc.want)
			}
		})
	}
}

// TestGoroutineHelpers is not parallel: it measures the process-wide
// goroutine count.
func TestGoroutineHelpers(t *testing.T) {
	base := tuitest.SettledGoroutines()
	release := make(chan struct{})
	go func() { <-release }()
	tuitest.Eventually(t, time.Second, func() bool { return runtime.NumGoroutine() > base }, "helper goroutine never started")
	if !strings.Contains(tuitest.Stacks("tuitest_test.TestGoroutineHelpers"), "TestGoroutineHelpers") {
		t.Error("Stacks did not find the parked goroutine")
	}
	tuitest.Eventually(t, time.Second, func() bool { return tuitest.Parked("tuitest_test.TestGoroutineHelpers") == 1 },
		"Parked should count the helper goroutine and skip the test goroutine")
	close(release)
	tuitest.WaitGoroutines(t, base, 2*time.Second)
	if n := tuitest.Parked("tuitest_test.TestGoroutineHelpers"); n != 0 {
		t.Errorf("Parked = %d after release", n)
	}

	fatals, _ := record(func(tb testing.TB) {
		tuitest.Eventually(tb, 20*time.Millisecond, func() bool { return false }, "never true")
	}).failures()
	if !anyContains(fatals, "never true") {
		t.Errorf("Eventually fatals = %q", fatals)
	}

	stuck := make(chan struct{})
	defer close(stuck)
	go func() { <-stuck }()
	fatals, _ = record(func(tb testing.TB) {
		tuitest.WaitGoroutines(tb, base, 20*time.Millisecond, "tuitest_test")
	}).failures()
	if !anyContains(fatals, "want at most") {
		t.Errorf("WaitGoroutines fatals = %q", fatals)
	}
}
