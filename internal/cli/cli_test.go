package cli

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/version"
)

func TestParse(t *testing.T) {
	t.Parallel()

	versionOut := fmt.Sprintf("k4s %s (%s %s/%s)\n", version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	usageParts := []string{"k4s ", "Usage:", "--version", "-h, --help", "K4S_THEME", "NO_COLOR"}

	cases := []struct {
		name        string
		args        []string
		wantProceed bool
		wantCode    int
		wantStdout  []string // substrings; nil means stdout must stay empty
		exactStdout string
		wantStderr  []string // substrings; nil means stderr must stay empty
	}{
		{name: "no args starts the tui", args: nil, wantProceed: true},
		{name: "double-dash version", args: []string{"--version"}, exactStdout: versionOut},
		{name: "single-dash version", args: []string{"-version"}, exactStdout: versionOut},
		{name: "version false starts the tui", args: []string{"--version=false"}, wantProceed: true},
		{name: "long help", args: []string{"--help"}, wantStdout: usageParts},
		{name: "short help", args: []string{"-h"}, wantStdout: usageParts},
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantCode:   ExitUsage,
			wantStderr: append([]string{"flag provided but not defined: -bogus"}, usageParts...),
		},
		{
			name:       "positional argument",
			args:       []string{"pods"},
			wantCode:   ExitUsage,
			wantStderr: append([]string{`unexpected argument "pods"`}, usageParts...),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			proceed, code := Parse(tc.args, &stdout, &stderr)

			if proceed != tc.wantProceed || code != tc.wantCode {
				t.Fatalf("Parse(%v) = (%v, %d), want (%v, %d)", tc.args, proceed, code, tc.wantProceed, tc.wantCode)
			}
			assertOutput(t, "stdout", stdout.String(), tc.exactStdout, tc.wantStdout)
			assertOutput(t, "stderr", stderr.String(), "", tc.wantStderr)
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestParseReportsFailedOutput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "version to a closed stdout", args: []string{"--version"}, wantCode: 1},
		{name: "help to a closed stdout", args: []string{"--help"}, wantCode: 1},
		{name: "usage error keeps its code", args: []string{"--bogus"}, wantCode: ExitUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proceed, code := Parse(tc.args, failingWriter{}, failingWriter{})
			if proceed || code != tc.wantCode {
				t.Fatalf("Parse(%v) = (%v, %d), want (false, %d)", tc.args, proceed, code, tc.wantCode)
			}
		})
	}
}

func assertOutput(t *testing.T, stream, got, exact string, parts []string) {
	t.Helper()
	if exact != "" {
		if got != exact {
			t.Errorf("%s = %q, want %q", stream, got, exact)
		}
		return
	}
	if parts == nil {
		if got != "" {
			t.Errorf("%s = %q, want empty", stream, got)
		}
		return
	}
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Errorf("%s missing %q:\n%s", stream, p, got)
		}
	}
}
