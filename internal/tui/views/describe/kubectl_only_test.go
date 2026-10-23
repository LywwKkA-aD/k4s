package describe_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

// TestKindOnlyKubectlDescribes: the built-in describer knows pods,
// deployments and services; a node or any other kind is kubectl's alone.
// When kubectl cannot describe it, the body and a warning say what is
// needed, and the footer keeps the kubectl describe command to copy.
func TestKindOnlyKubectlDescribes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mode    string
		found   bool
		timeout time.Duration
		none    bool // the view is given no kubectl at all
		want    string
	}{
		{name: "kubectl missing", mode: "ok", want: "describe node needs kubectl (not found on PATH)"},
		{name: "no kubectl given", none: true, want: "describe node needs kubectl"},
		{name: "kubectl fails", mode: "fail", found: true,
			want: `kubectl describe node failed: kubectl exited with status 1: Error from server (Forbidden): pods "web-1" is forbidden`},
		{name: "kubectl hangs", mode: "sleep", found: true, timeout: 300 * time.Millisecond, want: "kubectl describe node timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var opts []describe.Option
			if !tc.none {
				f := fakeKubectl(t, tc.mode, tc.found)
				f.cfg.Timeout = tc.timeout
				opts = append(opts, describe.WithKubectl(describe.NewKubectl(kubectl.NewRunner(f.cfg), decorate)))
			}
			m := describe.New(tuitest.NewFakeClient(t), "node", "", "k3s-server", opts...)
			m, _ = tuitest.Drive(t, m, tuitest.Resize(120, 40))
			m, msgs := tuitest.InitWith(t, driver, m)
			st, ok := statusOf(msgs)
			if !ok || st.StatusMsg != (views.StatusMsg{Level: status.Warn, Text: tc.want}) {
				t.Errorf("status = %+v (found %v), want a warning %q", st, ok, tc.want)
			}
			if got := st.Cmd.String(); got != "kubectl describe node k3s-server" {
				t.Errorf("warning is about %q, want the describe it needs kubectl for", got)
			}
			if screen := tuitest.Screen(m); !strings.Contains(screen, tc.want) || strings.Contains(screen, "unsupported") {
				t.Errorf("body:\n%s", screen)
			}
			if c := m.Command(); c.String() != "kubectl describe node k3s-server" || c.Approx {
				t.Errorf("Command = %q (approx %v)", c.String(), c.Approx)
			}
			if got := m.Title(); got != "node · k3s-server" {
				t.Errorf("Title = %q", got)
			}
		})
	}
}

// TestKubectlDescribesANode: with kubectl there, a node is described as
// any other kind.
func TestKubectlDescribesANode(t *testing.T) {
	t.Parallel()

	f := fakeKubectl(t, "ok", true)
	k := describe.NewKubectl(kubectl.NewRunner(f.cfg), decorate)
	m := describe.New(tuitest.NewFakeClient(t), "node", "", "k3s-server", describe.WithKubectl(k))
	m, msgs := tuitest.InitWith(t, driver, m)
	if st, ok := statusOf(msgs); ok {
		t.Errorf("a clean run said %+v", st)
	}
	if !strings.Contains(tuitest.Screen(m), "from-kubectl") {
		t.Errorf("screen:\n%s", tuitest.Screen(m))
	}
	if got, want := f.ranArgv(t), decorate(m.Command()).Argv()[1:]; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("kubectl ran %q, want %q", got, want)
	}
}
