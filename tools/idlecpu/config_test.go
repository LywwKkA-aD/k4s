package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

// testExe is an executable that parseConfig accepts as --bin.
func testExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("find the test binary: %v", err)
	}
	return exe
}

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()

	exe := testExe(t)
	got, err := parseConfig([]string{"--bin", exe}, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	want := config{
		bin:         exe,
		duration:    40 * time.Second,
		warmup:      3 * time.Second,
		interval:    time.Second,
		cols:        120,
		rows:        40,
		term:        "xterm-256color",
		churn:       true,
		kubectl:     "kubectl",
		quitKeys:    "q",
		quitTimeout: 5 * time.Second,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %+v\nwant     %+v", got, want)
	}
}

func TestParseConfigFlags(t *testing.T) {
	t.Parallel()

	exe := testExe(t)
	kubeconfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(kubeconfig, []byte("apiVersion: v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--bin", exe, "--kubeconfig", kubeconfig,
		"--duration", "10s", "--warmup", "500ms", "--interval", "250ms", "--size", "80x24",
		"--term", "tmux-256color", "--env", "K4S_THEME=mono", "--env", "COLORTERM=truecolor",
		"--max-cpu", "2", "--max-rss", "40MB", "--require-idle", "--churn=false",
		"--kubectl", "k3s-kubectl", "--quit-timeout", "2s", "--json",
		"--", "-n", "k4s-demo",
	}
	got, err := parseConfig(args, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	want := config{
		bin:         exe,
		args:        []string{"-n", "k4s-demo"},
		kubeconfig:  kubeconfig,
		duration:    10 * time.Second,
		warmup:      500 * time.Millisecond,
		interval:    250 * time.Millisecond,
		cols:        80,
		rows:        24,
		term:        "tmux-256color",
		env:         []string{"K4S_THEME=mono", "COLORTERM=truecolor"},
		gates:       gates{maxCPU: 2, maxRSS: 40_000_000, requireIdle: true},
		churn:       false,
		kubectl:     "k3s-kubectl",
		quitKeys:    "q",
		quitTimeout: 2 * time.Second,
		json:        true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %+v\nwant     %+v", got, want)
	}
}

func TestParseConfigErrors(t *testing.T) {
	t.Parallel()

	exe := testExe(t)
	dir := t.TempDir()
	notExec := filepath.Join(dir, "k4s")
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "binary missing", args: []string{"--bin", filepath.Join(dir, "nope")}, wantErr: "stat binary"},
		{name: "binary is a directory", args: []string{"--bin", dir}, wantErr: "not an executable file"},
		{name: "binary not executable", args: []string{"--bin", notExec}, wantErr: "not an executable file"},
		{name: "zero duration", args: []string{"--bin", exe, "--duration", "0s"}, wantErr: "--duration"},
		{name: "negative warm-up", args: []string{"--bin", exe, "--warmup", "-1s"}, wantErr: "--warmup"},
		{name: "zero interval", args: []string{"--bin", exe, "--interval", "0s"}, wantErr: "--interval"},
		{name: "interval over the window", args: []string{"--bin", exe, "--duration", "1s", "--interval", "2s"}, wantErr: "--interval"},
		{name: "zero quit timeout", args: []string{"--bin", exe, "--quit-timeout", "0s"}, wantErr: "--quit-timeout"},
		{name: "bad size", args: []string{"--bin", exe, "--size", "big"}, wantErr: "want COLSxROWS"},
		{name: "bare rss limit", args: []string{"--bin", exe, "--max-rss", "40"}, wantErr: "want a unit"},
		{name: "negative cpu limit", args: []string{"--bin", exe, "--max-cpu", "-1"}, wantErr: "--max-cpu"},
		{name: "env without a value", args: []string{"--bin", exe, "--env", "K4S_THEME"}, wantErr: "want KEY=VALUE"},
		{name: "env without a key", args: []string{"--bin", exe, "--env", "=mono"}, wantErr: "want KEY=VALUE"},
		{name: "empty term", args: []string{"--bin", exe, "--term", ""}, wantErr: "--term"},
		{name: "empty quit keys", args: []string{"--bin", exe, "--quit-keys", ""}, wantErr: "--quit-keys"},
		{name: "kubeconfig missing", args: []string{"--bin", exe, "--kubeconfig", filepath.Join(dir, "none")}, wantErr: "stat kubeconfig"},
		{name: "unknown flag", args: []string{"--bin", exe, "--bogus"}, wantErr: "flag provided but not defined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseConfig(tc.args, io.Discard)
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestUnquoteKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want, wantErr string
	}{
		{in: "q", want: "q"},
		{in: `\x03`, want: "\x03"},
		{in: `q\n`, want: "q\n"},
		{in: `say "q"`, want: `say "q"`},
		{in: `\z`, wantErr: "parse --quit-keys"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := unquoteKeys(tc.in)
			checkErr(t, err, tc.wantErr)
			if got != tc.want {
				t.Fatalf("unquoteKeys(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseConfigHelp(t *testing.T) {
	t.Parallel()

	if _, err := parseConfig([]string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
}

func TestChildEnv(t *testing.T) {
	t.Parallel()

	parent := []string{
		"PATH=/usr/bin", "HOME=/home/pi", "TERM=alacritty", "COLORTERM=truecolor",
		"TERM_PROGRAM=iTerm.app", "TERM_PROGRAM_VERSION=3", "TMUX=/tmp/tmux-1000/default,1,0",
		"TMUX_PANE=%1", "NO_COLOR=1", "K4S_THEME=light", "KUBECONFIG=/home/pi/.kube/other",
		"NOEQUALS",
	}
	cases := []struct {
		name       string
		kubeconfig string
		extra      []string
		want       []string
	}{
		{
			name: "terminal identity dropped, kubeconfig inherited",
			want: []string{"PATH=/usr/bin", "HOME=/home/pi", "KUBECONFIG=/home/pi/.kube/other", "NOEQUALS", "TERM=xterm-256color"},
		},
		{
			name:       "explicit kubeconfig and extra env last",
			kubeconfig: ".kube/config",
			extra:      []string{"K4S_THEME=mono"},
			want: []string{
				"PATH=/usr/bin", "HOME=/home/pi", "NOEQUALS",
				"TERM=xterm-256color", "KUBECONFIG=.kube/config", "K4S_THEME=mono",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := childEnv(slices.Clone(parent), "xterm-256color", tc.kubeconfig, tc.extra)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("env = %q\nwant  %q", got, tc.want)
			}
		})
	}
}
