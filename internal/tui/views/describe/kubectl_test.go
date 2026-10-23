package describe_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

// decorate stands in for the root's kubectl.Decorate: the flags a pasted
// copy needs, which the run must carry too.
func decorate(c kubectl.Cmd) kubectl.Cmd {
	return c.WithContext("prod").WithKubeconfig([]string{"/etc/k4s/kubeconfig"})
}

// driver waits long enough for a real subprocess.
var driver = tuitest.Driver{CmdTimeout: 15 * time.Second}

func mounted(t *testing.T, f fake) (describe.Model, []tea.Msg) {
	t.Helper()
	k := describe.NewKubectl(kubectl.NewRunner(f.cfg), decorate)
	m := describe.New(tuitest.NewFakeClient(t), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb1, describe.WithKubectl(k))
	m, _ = tuitest.Drive(t, m, tuitest.Resize(120, 40))
	return tuitest.InitWith(t, driver, m)
}

// statusOf is the status the view sent: a warning about its describe
// command, or a plain status.
func statusOf(msgs []tea.Msg) (views.StatusAboutMsg, bool) {
	if st, ok := tuitest.Find[views.StatusAboutMsg](msgs); ok {
		return st, true
	}
	st, ok := tuitest.Find[views.StatusMsg](msgs)
	return views.StatusAboutMsg{StatusMsg: st}, ok
}

// TestKubectlRunsTheFooterCommand: the argv kubectl gets is the footer's
// command, decorated as the footer shows it, word for word.
func TestKubectlRunsTheFooterCommand(t *testing.T) {
	t.Parallel()

	f := fakeKubectl(t, "ok", true)
	m, msgs := mounted(t, f)
	want := decorate(m.Command()).Argv()[1:]
	if got := f.ranArgv(t); !slices.Equal(got, want) {
		t.Errorf("kubectl ran %q\nfooter shows %q", got, want)
	}
	if !strings.Contains(tuitest.Screen(m), "from-kubectl") {
		t.Errorf("the kubectl output is not shown:\n%s", tuitest.Screen(m))
	}
	if got, want := m.Title(), "pod · web-1 · via kubectl "+fakeVersion; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if m.Command().Approx {
		t.Error("the footer marks kubectl's own output approximate")
	}
	if st, ok := statusOf(msgs); ok {
		t.Errorf("a clean run said %+v", st)
	}
}

func TestKubectlStderrIsIgnoredOnSuccess(t *testing.T) {
	t.Parallel()

	m, msgs := mounted(t, fakeKubectl(t, "warn", true))
	if st, ok := statusOf(msgs); ok {
		t.Errorf("a warning on exit 0 reached the status row: %+v", st)
	}
	if screen := tuitest.Screen(m); strings.Contains(screen, "deprecated") || !strings.Contains(screen, "from-kubectl") {
		t.Errorf("screen:\n%s", screen)
	}
}

func TestKubectlOutputIsMadeInert(t *testing.T) {
	t.Parallel()

	m, _ := mounted(t, fakeKubectl(t, "escape", true))
	frame := tuitest.Frame(m)
	for _, bad := range []string{"\x1b]52", "\x1b[2J", "\r"} {
		if strings.Contains(frame, bad) {
			t.Errorf("the frame carries %q from kubectl:\n%q", bad, frame)
		}
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "evil") || !strings.Contains(screen, "Next: line") {
		t.Errorf("the text around the escapes is lost:\n%s", screen)
	}
}

// TestFallbackToBuiltIn: whenever kubectl cannot answer, the built-in
// describer does, the title says so, the footer marks the command
// approximate and the status row says why.
func TestFallbackToBuiltIn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mode    string
		found   bool
		timeout time.Duration
		level   status.Level
		reason  string
	}{
		{name: "kubectl missing", mode: "ok", found: false, level: status.Info, reason: "kubectl not found on PATH"},
		{name: "kubectl fails", mode: "fail", found: true, level: status.Warn, reason: `Error from server (Forbidden): pods "web-1" is forbidden`},
		{name: "kubectl hangs", mode: "sleep", found: true, timeout: 300 * time.Millisecond, level: status.Warn, reason: "kubectl timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := fakeKubectl(t, tc.mode, tc.found)
			f.cfg.Timeout = tc.timeout
			m, msgs := mounted(t, f)
			if screen := tuitest.Screen(m); !strings.Contains(screen, "Containers:") || strings.Contains(screen, "from-kubectl") {
				t.Errorf("the built-in describe is not shown:\n%s", screen)
			}
			if got, want := m.Title(), "pod · web-1 · built-in"; got != want {
				t.Errorf("Title = %q, want %q", got, want)
			}
			if !m.Command().Approx {
				t.Error("the footer does not mark the built-in output approximate")
			}
			st, ok := statusOf(msgs)
			if !ok || st.Level != tc.level || !strings.Contains(st.Text, tc.reason) || !strings.Contains(st.Text, "built-in") {
				t.Errorf("status = %+v (found %v), want %v naming %q", st, ok, tc.level, tc.reason)
			}
			if got, want := st.Cmd.String(), m.Command().String(); got != want {
				t.Errorf("status is about %q, want the describe %q", got, want)
			}
		})
	}
}

func TestKubectlVersionOncePerSession(t *testing.T) {
	t.Parallel()

	f := fakeKubectl(t, "ok", true)
	k := describe.NewKubectl(kubectl.NewRunner(f.cfg), decorate)
	c := tuitest.NewFakeClient(t)
	for _, pod := range []string{tuitest.PodWeb1, tuitest.PodWeb2} {
		m := describe.New(c, describe.KindPod, tuitest.NamespaceDemo, pod, describe.WithKubectl(k))
		m, _ = tuitest.InitWith(t, driver, m)
		if !strings.Contains(m.Title(), "via kubectl "+fakeVersion) {
			t.Errorf("Title = %q", m.Title())
		}
	}
	if n := f.versionCalls(t); n != 1 {
		t.Errorf("kubectl version ran %d times, want once", n)
	}
}

// TestKubectlVersionInTheTitle: the version is kubectl's own word, made
// inert and kept short; without one the title says kubectl alone.
func TestKubectlVersionInTheTitle(t *testing.T) {
	t.Parallel()

	cases := []struct{ mode, want string }{
		{mode: "noversion", want: "pod · web-1 · via kubectl"},
		{mode: "garbledversion", want: "pod · web-1 · via kubectl"},
		{mode: "blankversion", want: "pod · web-1 · via kubectl"},
		{mode: "longversion", want: "pod · web-1 · via kubectl v1.33.9+k3sk3sk3sk3sk3sk3sk3sk3s"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()
			m, _ := mounted(t, fakeKubectl(t, tc.mode, true))
			if got := m.Title(); got != tc.want {
				t.Errorf("Title = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestKubectlRefusesAnInvalidName: a namespace kubectl would misread
// never reaches it; the built-in describer answers and the status says
// why.
func TestKubectlRefusesAnInvalidName(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, namespace, pod string }{
		{name: "namespace", namespace: "-A", pod: tuitest.PodWeb1},
		{name: "pod", namespace: tuitest.NamespaceDemo, pod: "--all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := fakeKubectl(t, "ok", true)
			k := describe.NewKubectl(kubectl.NewRunner(f.cfg), decorate)
			m := describe.New(tuitest.NewFakeClient(t), describe.KindPod, tc.namespace, tc.pod, describe.WithKubectl(k))
			_, msgs := tuitest.InitWith(t, driver, m)
			if got := f.ranArgv(t); got != nil {
				t.Errorf("kubectl ran %q", got)
			}
			st, ok := statusOf(msgs)
			if !ok || st.Level != status.Warn || !strings.Contains(st.Text, "built-in describe: invalid name") {
				t.Errorf("status = %+v (found %v)", st, ok)
			}
		})
	}
}
