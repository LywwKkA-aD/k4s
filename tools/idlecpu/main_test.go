package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	t.Parallel()

	exe := testExe(t)
	fake := []string{
		"--bin", exe, "--duration", "300ms", "--warmup", "100ms", "--interval", "100ms",
		"--churn=false", "--env", fakeTUIEnv + "=draw", "--quit-keys", `q\n`,
	}
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "help", args: []string{"--help"}, wantCode: exitOK, wantStderr: "Usage of idlecpu"},
		{name: "bad flag", args: []string{"--bogus"}, wantCode: exitError, wantStderr: "idlecpu: "},
		{name: "measured, no gate", args: fake, wantCode: exitOK, wantStdout: "result   PASS"},
		{
			name:       "gate passes",
			args:       append([]string{"--max-cpu", "1000", "--max-rss", "1GiB"}, fake...),
			wantCode:   exitOK,
			wantStdout: "gate     rss",
		},
		{
			name:       "rss gate fails",
			args:       append([]string{"--max-rss", "1KiB"}, fake...),
			wantCode:   exitGate,
			wantStdout: "result   FAIL",
		},
		{
			name:       "idle cannot be proven without the pod check",
			args:       append([]string{"--require-idle"}, fake...),
			wantCode:   exitGate,
			wantStdout: "not checked (--churn=false)",
		},
		{
			name:       "harness error",
			args:       []string{"--bin", exe, "--interval", "100ms", "--duration", "300ms", "--churn=false", "--env", fakeTUIEnv + "=exit-early"},
			wantCode:   exitError,
			wantStderr: "exited during the warm-up",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout lacks %q:\n%s", tc.wantStdout, stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr lacks %q:\n%s", tc.wantStderr, stderr.String())
			}
		})
	}
}

func TestRunJSON(t *testing.T) {
	t.Parallel()

	args := []string{
		"--bin", testExe(t), "--duration", "300ms", "--warmup", "100ms", "--interval", "100ms",
		"--churn=false", "--env", fakeTUIEnv + "=draw", "--quit-keys", `q\n`, "--json",
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d\nstderr:\n%s", code, stderr.String())
	}
	var got report
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a JSON report: %v\n%s", err, stdout.String())
	}
	if !got.Pass || got.Samples < 2 || got.Quit != "exit 0 on q" {
		t.Fatalf("report = %+v", got)
	}
}
