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

// env is a getenv over a fixed map.
func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestParseExits(t *testing.T) {
	t.Parallel()

	versionOut := fmt.Sprintf("k4s %s (%s %s/%s)\n", version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	usageParts := []string{
		"k4s ", "Usage:", "--version", "-h, --help", "--kubeconfig", "--context",
		"-n, --namespace", "-A, --all-namespaces", "--theme", "--no-preflight", "--no-recap",
		"KUBECONFIG", "K4S_THEME", "K4S_NO_PREFLIGHT", "K4S_NO_RECAP", "NO_COLOR", "/etc/rancher/k3s/k3s.yaml",
	}

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
		{name: "version after a stray argument", args: []string{"pods", "--version"}, exactStdout: versionOut},
		{name: "long help", args: []string{"--help"}, wantStdout: usageParts},
		{name: "short help", args: []string{"-h"}, wantStdout: usageParts},
		{
			name:       "unknown flag",
			args:       []string{"--bogus"},
			wantCode:   ExitUsage,
			wantStderr: append([]string{"flag provided but not defined: -bogus"}, usageParts...),
		},
		{
			name:       "unknown theme",
			args:       []string{"--theme", "purple"},
			wantCode:   ExitUsage,
			wantStderr: []string{`invalid value "purple" for flag -theme: want auto, dark, light, mono or high-contrast`, "Usage:"},
		},
		{
			name:       "namespace that is not a DNS-1123 label",
			args:       []string{"-n", "Team_A"},
			wantCode:   ExitUsage,
			wantStderr: []string{`invalid value "Team_A" for flag -n: invalid namespace "Team_A"`, "Usage:"},
		},
		{
			name:       "flag value missing",
			args:       []string{"--context"},
			wantCode:   ExitUsage,
			wantStderr: []string{"flag needs an argument: -context", "Usage:"},
		},
		// Earlier releases ignored stray arguments, and so does this one.
		{name: "positional argument is ignored", args: []string{"pods"}, wantProceed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			_, proceed, code := Parse(tc.args, env(nil), &stdout, &stderr)

			if proceed != tc.wantProceed || code != tc.wantCode {
				t.Fatalf("Parse(%v) = (%v, %d), want (%v, %d)", tc.args, proceed, code, tc.wantProceed, tc.wantCode)
			}
			assertOutput(t, "stdout", stdout.String(), tc.exactStdout, tc.wantStdout)
			assertOutput(t, "stderr", stderr.String(), "", tc.wantStderr)
		})
	}
}

func TestParseOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		env  map[string]string
		want Options
	}{
		{name: "defaults", want: Options{}},
		{
			name: "kubeconfig and context",
			args: []string{"--kubeconfig", "/tmp/kc", "--context=prod"},
			want: Options{Kubeconfig: "/tmp/kc", Context: "prod"},
		},
		{name: "short namespace", args: []string{"-n", "web"}, want: Options{Namespace: "web"}},
		{name: "long namespace", args: []string{"--namespace=web"}, want: Options{Namespace: "web"}},
		{name: "empty namespace means not given", args: []string{"-n", ""}, want: Options{}},
		{name: "short all namespaces", args: []string{"-A"}, want: Options{AllNamespaces: true}},
		{name: "long all namespaces", args: []string{"--all-namespaces"}, want: Options{AllNamespaces: true}},
		{
			name: "both namespace flags are kept",
			args: []string{"-A", "-n", "web"},
			want: Options{Namespace: "web", AllNamespaces: true},
		},
		{name: "theme is normalised", args: []string{"--theme", " Light "}, want: Options{Theme: "light"}},
		{name: "high-contrast theme", args: []string{"--theme=High-Contrast"}, want: Options{Theme: "high-contrast"}},
		{name: "no-preflight flag", args: []string{"--no-preflight"}, want: Options{NoPreflight: true}},
		{name: "no-preflight from env", env: map[string]string{"K4S_NO_PREFLIGHT": "1"}, want: Options{NoPreflight: true}},
		{name: "env any other value", env: map[string]string{"K4S_NO_PREFLIGHT": "yes"}, want: Options{NoPreflight: true}},
		{name: "env zero", env: map[string]string{"K4S_NO_PREFLIGHT": "0"}, want: Options{}},
		{name: "env false", env: map[string]string{"K4S_NO_PREFLIGHT": " FALSE "}, want: Options{}},
		{
			name: "flag beats env",
			args: []string{"--no-preflight=false"},
			env:  map[string]string{"K4S_NO_PREFLIGHT": "1"},
			want: Options{},
		},
		{name: "no-recap flag", args: []string{"--no-recap"}, want: Options{NoRecap: true}},
		{name: "no-recap from env", env: map[string]string{"K4S_NO_RECAP": "1"}, want: Options{NoRecap: true}},
		{name: "no-recap env off", env: map[string]string{"K4S_NO_RECAP": "off"}, want: Options{}},
		{
			name: "no-recap flag beats env",
			args: []string{"--no-recap=false"},
			env:  map[string]string{"K4S_NO_RECAP": "true"},
			want: Options{},
		},
		{
			name: "the two switches are separate",
			env:  map[string]string{"K4S_NO_PREFLIGHT": "1"},
			args: []string{"--no-recap"},
			want: Options{NoPreflight: true, NoRecap: true},
		},
		// kubectl users type a resource first; flags after it still count.
		{
			name: "flags after a stray argument",
			args: []string{"pods", "-n", "web", "extra", "-A"},
			want: Options{Namespace: "web", AllNamespaces: true},
		},
		{name: "everything after -- is positional", args: []string{"--", "-n", "web"}, want: Options{}},
		{
			name: "-- after a stray argument",
			args: []string{"pods", "--context", "dev", "--", "-A"},
			want: Options{Context: "dev"},
		},
		{
			name: "-- as a flag value is not the terminator",
			args: []string{"--kubeconfig", "--", "pods", "-n", "web"},
			want: Options{Kubeconfig: "--", Namespace: "web"},
		},
		{
			name: "-- as the context is not the terminator",
			args: []string{"--context", "--", "pods", "-A"},
			want: Options{Context: "--", AllNamespaces: true},
		},
		{
			name: "-- after a value-taking flag with = is the terminator",
			args: []string{"--context=dev", "--", "-A"},
			want: Options{Context: "dev"},
		},
		{
			name: "-- after a bool flag is the terminator",
			args: []string{"--no-preflight", "--", "-A"},
			want: Options{NoPreflight: true},
		},
		{
			name: "a -- value then the real terminator",
			args: []string{"--kubeconfig", "--", "--", "-A"},
			want: Options{Kubeconfig: "--"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			got, proceed, code := Parse(tc.args, env(tc.env), &stdout, &stderr)
			if !proceed || code != 0 {
				t.Fatalf("Parse(%v) = (%v, %d), stderr %q; want to proceed", tc.args, proceed, code, stderr.String())
			}
			if got != tc.want {
				t.Fatalf("Parse(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output: stdout %q stderr %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestOptionsGetenv(t *testing.T) {
	t.Parallel()

	base := env(map[string]string{"K4S_THEME": "dark", "NO_COLOR": "1", "COLORTERM": "truecolor"})
	cases := []struct {
		name  string
		theme string
		key   string
		want  string
	}{
		{name: "no flag keeps K4S_THEME", key: "K4S_THEME", want: "dark"},
		{name: "no flag keeps NO_COLOR", key: "NO_COLOR", want: "1"},
		{name: "flag replaces K4S_THEME", theme: "light", key: "K4S_THEME", want: "light"},
		// An explicit flag beats NO_COLOR (https://no-color.org).
		{name: "flag hides NO_COLOR", theme: "light", key: "NO_COLOR", want: ""},
		{name: "other keys pass through", theme: "light", key: "COLORTERM", want: "truecolor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Options{Theme: tc.theme}.Getenv(base)(tc.key)
			if got != tc.want {
				t.Fatalf("Getenv(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestThemesAreDocumented(t *testing.T) {
	t.Parallel()

	for _, th := range Themes() {
		if !strings.Contains(usage, th) {
			t.Errorf("usage does not mention theme %q", th)
		}
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
			_, proceed, code := Parse(tc.args, env(nil), failingWriter{}, failingWriter{})
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
