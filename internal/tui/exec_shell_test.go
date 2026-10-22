package tui

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func windowsPod(name string, byNodeSelector bool) *corev1.Pod {
	p := tuitest.Pod(tuitest.NamespaceDemo, name, nil, "app")
	if byNodeSelector {
		p.Spec.NodeSelector = map[string]string{corev1.LabelOSStable: "windows"}
		return p
	}
	p.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
	return p
}

func TestShellFor(t *testing.T) {
	t.Parallel()

	linux := tuitest.Pod(tuitest.NamespaceDemo, "web", nil, "app")
	linux.Spec.OS = &corev1.PodOS{Name: corev1.Linux}
	cases := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{name: "no pod", pod: nil, want: "sh"},
		{name: "plain pod", pod: tuitest.Pod(tuitest.NamespaceDemo, "web", nil, "app"), want: "sh"},
		{name: "linux by spec.os", pod: linux, want: "sh"},
		{name: "windows by spec.os", pod: windowsPod("win", false), want: "cmd.exe"},
		{name: "windows by node selector", pod: windowsPod("win", true), want: "cmd.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shellFor(tc.pod); got != tc.want {
				t.Errorf("shellFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExecArgvPerShell: sh is exactly the footer's command; a Windows pod
// differs only in the shell.
func TestExecArgvPerShell(t *testing.T) {
	t.Parallel()

	req := views.ExecRequestMsg{Namespace: "shop", Pod: "api-7d9f", Container: "app"}
	m := withLookPath(New(nil, nil), foundAt(fakeKubectlPath), nil)
	footer := mustExec(t, req).Argv()
	cases := []struct {
		shell string
		want  []string
	}{
		{shell: "sh", want: append([]string{fakeKubectlPath}, footer[1:]...)},
		{shell: "cmd.exe", want: []string{fakeKubectlPath, "exec", "-it", "-n", "shop", "-c", "app", "api-7d9f", "--", "cmd.exe"}},
	}
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			t.Parallel()
			c, err := m.execCommand(req, tc.shell)
			if err != nil {
				t.Fatalf("execCommand: %v", err)
			}
			if !slices.Equal(c.Args, tc.want) {
				t.Errorf("argv = %q, want %q", c.Args, tc.want)
			}
		})
	}
}

// TestExecRunsTheShellTheRequestNames: a request that names its shell, as
// the pods view's lookup found it, starts at once, with no second GET.
func TestExecRunsTheShellTheRequestNames(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{views.ExecShell, views.WindowsShell} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			var gets atomic.Int32
			count := tuitest.WithReactor("get", "pods", func(k8stesting.Action) (bool, kruntime.Object, error) {
				gets.Add(1)
				return false, nil, nil
			})
			m := withLookPath(New(tuitest.NewFakeClient(t, count), nil), foundAt(fakeKubectlPath), nil)
			req := views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1, Container: "app", Shell: shell}
			_, cmd := m.Update(req)
			if cmd == nil || !slices.ContainsFunc(tuitest.Collect(t, cmd), tuitest.IsExec) {
				t.Fatal("the exec did not start")
			}
			if n := gets.Load(); n != 0 {
				t.Errorf("the root looked the pod up %d times", n)
			}
		})
	}
}

// resolve sends req through the root and runs the pod lookup it starts.
func resolve(t *testing.T, m Model, req views.ExecRequestMsg) execResolvedMsg {
	t.Helper()
	_, cmd := m.Update(req)
	if cmd == nil {
		t.Fatal("exec request started no lookup")
	}
	msg, ok := cmd().(execResolvedMsg)
	if !ok {
		t.Fatalf("lookup answered %T", msg)
	}
	return msg
}

func TestExecLooksThePodUp(t *testing.T) {
	t.Parallel()

	refuse := tuitest.WithReactor("get", "pods", func(k8stesting.Action) (bool, kruntime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "web-1", errors.New("no"))
	})
	cases := []struct {
		name      string
		client    *k8s.Client
		pod       string
		wantShell string
		wantExec  bool
		wantText  string
	}{
		{name: "linux pod", client: tuitest.NewFakeClient(t), pod: tuitest.PodWeb1, wantShell: "sh", wantExec: true},
		{
			name:   "windows pod",
			client: tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(tuitest.NamespaceDemo), windowsPod("win-0", false))),
			pod:    "win-0", wantShell: "cmd.exe", wantExec: true,
		},
		{name: "lookup refused", client: tuitest.NewFakeClient(t, refuse), pod: tuitest.PodWeb1, wantShell: "sh", wantExec: true},
		{name: "pod gone", client: tuitest.NewFakeClient(t), pod: "ghost", wantText: "exec: pod demo/ghost is gone, press r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := withLookPath(New(tc.client, nil), foundAt(fakeKubectlPath), nil)
			res := resolve(t, m, views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tc.pod, Container: "app"})
			if res.err == nil && shellFor(res.pod) != tc.wantShell {
				t.Errorf("shell = %q, want %q", shellFor(res.pod), tc.wantShell)
			}
			next, cmd := m.Update(res)
			if got := cmd != nil && slices.ContainsFunc(tuitest.Collect(t, cmd), tuitest.IsExec); got != tc.wantExec {
				t.Errorf("exec started = %v, want %v", got, tc.wantExec)
			}
			if tc.wantText == "" {
				return
			}
			if msg, ok := next.(Model).status.Current(time.Now()); !ok || msg.Text != tc.wantText {
				t.Errorf("status = %+v, want %q", msg, tc.wantText)
			}
		})
	}
}

// TestExecLookupAfterAContextSwitch: the pod was in the old cluster.
func TestExecLookupAfterAContextSwitch(t *testing.T) {
	t.Parallel()

	m := withLookPath(New(tuitest.NewFakeClient(t, tuitest.WithContext("new")), nil), foundAt(fakeKubectlPath), nil)
	stale := execResolvedMsg{context: "old", req: views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1}}
	if _, cmd := m.Update(stale); cmd != nil {
		t.Error("a lookup from before :ctx started an exec")
	}
}

// exitErr is a real *exec.ExitError with code.
func exitErr(t *testing.T, code string) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	err := exec.Command("sh", "-c", "exit "+code).Run() //nolint:gosec // test drives a fixed shell
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("sh -c 'exit %s' = %v", code, err)
	}
	return err
}

// TestShellDoneHintsAtKubectlDebug is M4.6: a container without the shell
// gets the kubectl debug command, shown and never run. The status row says
// so; the command is the one the status is about, for the command row.
func TestShellDoneHintsAtKubectlDebug(t *testing.T) {
	t.Parallel()

	web := views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1, Container: "app"}
	noShell := "no sh in demo/web-1 · y copies the kubectl debug command"
	debug := "kubectl debug -it -n demo web-1 --image=busybox:1.36 --target=app"
	cases := []struct {
		name string
		done shellDoneMsg
		want string // "" means no status
		cmd  string // the command the status is about; "" means none
	}{
		{name: "clean exit", done: shellDoneMsg{req: web, shell: "sh"}},
		{
			name: "127 at once", done: shellDoneMsg{req: web, shell: "sh", err: exitErr(t, "127"), elapsed: time.Second},
			want: noShell + " · exit 127", cmd: debug,
		},
		{
			name: "126 at once", done: shellDoneMsg{req: web, shell: "sh", err: exitErr(t, "126"), elapsed: time.Second},
			want: noShell + " · exit 126", cmd: debug,
		},
		{
			name: "the runtime says so", done: shellDoneMsg{req: web, shell: "sh", err: exitErr(t, "1"), elapsed: time.Minute,
				stderr: `OCI runtime exec failed: exec: "sh": executable file not found in $PATH: unknown`},
			want: noShell + " · exit 1", cmd: debug,
		},
		{
			name: "127 after a session is the user's last command",
			done: shellDoneMsg{req: web, shell: "sh", err: exitErr(t, "127"), elapsed: time.Minute},
			want: "exec: exit status 127",
		},
		{
			name: "another failure", done: shellDoneMsg{req: web, shell: "sh", err: exitErr(t, "1"), elapsed: time.Second},
			want: "exec: exit status 1",
		},
		{
			name: "not an exit", done: shellDoneMsg{req: web, shell: "sh", err: errors.New("exec: executable file not found in $PATH")},
			want: noShell, cmd: debug,
		},
		{
			name: "no container named", done: shellDoneMsg{req: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1"}, shell: "sh", err: exitErr(t, "127")},
			want: noShell + " · exit 127", cmd: "kubectl debug -it -n demo web-1 --image=busybox:1.36",
		},
		{
			name: "windows has no busybox", done: shellDoneMsg{req: web, shell: "cmd.exe", err: exitErr(t, "127")},
			want: "exec: no cmd.exe in web-1/app · exit 127",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next, _ := New(nil, nil).Update(selfApplyingMsg(tc.done))
			line := next.(Model).status
			msg, ok := line.Current(time.Now())
			switch {
			case tc.want == "" && ok:
				t.Errorf("status = %+v after a clean exit", msg)
			case tc.want != "" && (!ok || msg.Text != tc.want):
				t.Errorf("status = %+v, %v; want %q", msg, ok, tc.want)
			}
			if cmd, _ := line.Command(time.Now()); cmd.String() != tc.cmd {
				t.Errorf("status command = %q, want %q", cmd.String(), tc.cmd)
			}
		})
	}
}

// commandRows are the footer's two rows: the status (or hints) and the
// kubectl command.
func commandRows(t *testing.T, m Model) (top, command string) {
	t.Helper()
	top, command, _ = strings.Cut(tuitest.Footer(m), "\n")
	return strings.TrimSpace(top), strings.TrimSpace(command)
}

// TestDebugHintFitsEightyColumns: the status row is cut at the width, so
// the kubectl debug command goes where commands go, the command row, while
// the status about it shows; y copies it from there, even as the key that
// dismisses the error, and the view's command comes back once the status
// goes.
func TestDebugHintFitsEightyColumns(t *testing.T) {
	t.Parallel()

	clk := &manualClock{at: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	m := New(nil, nil, WithClock(clk.now))
	m.width, m.height = 80, 24
	_, view := commandRows(t, m)
	done := shellDoneMsg{
		req:   views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1, Container: tuitest.ContainerNginx},
		shell: "sh", err: errors.New(`exec: "sh": executable file not found in $PATH`),
	}
	want := "kubectl debug -it -n demo web-1 --image=busybox:1.36 --target=nginx"

	m, _ = tuitest.Drive(t, m, selfApplyingMsg(done))
	top, command := commandRows(t, m)
	if top != styles.GlyphErr+" no sh in demo/web-1 · y copies the kubectl debug command" || command != want {
		t.Fatalf("footer at 80 columns:\n%s\n%s\nwant the command row %q", top, command, want)
	}

	clk.at = clk.at.Add(time.Minute)
	m, out := tuitest.Drive(t, m, tuitest.Press("y"))
	if got, ok := tuitest.Clipboard(out); !ok || got != want {
		t.Fatalf("y copied %q, %v; want %q", got, ok, want)
	}
	if _, command = commandRows(t, m); command != want {
		t.Errorf("command row while the copy shows = %q, want %q", command, want)
	}

	clk.at = clk.at.Add(status.TTL)
	if _, command = commandRows(t, m); command != view {
		t.Errorf("command row once the status went = %q, want the view's %q", command, view)
	}
}

// TestDebugCommandLeavesWithItsStatus: whatever takes the status away
// gives the command row back to the view.
func TestDebugCommandLeavesWithItsStatus(t *testing.T) {
	t.Parallel()

	done := shellDoneMsg{
		req:   views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1, Container: tuitest.ContainerNginx},
		shell: "sh", err: errors.New(`exec: "sh": executable file not found in $PATH`),
	}
	debug := "kubectl debug -it -n demo web-1 --image=busybox:1.36 --target=nginx"
	cases := []struct {
		name   string
		before time.Duration // the clock moves on, then x is pressed
		after  time.Duration // then the clock moves on again
		debug  bool
	}{
		{name: "a key inside the ttl", before: time.Second, debug: true},
		{name: "the ttl after a key inside it", before: time.Second, after: status.TTL},
		{name: "a key after the ttl", before: time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := &manualClock{at: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
			m := New(nil, nil, WithClock(clk.now))
			m.width, m.height = 80, 24
			_, view := commandRows(t, m)
			m, _ = tuitest.Drive(t, m, selfApplyingMsg(done))
			clk.at = clk.at.Add(tc.before)
			m, _ = tuitest.Drive(t, m, tuitest.Press("x"))
			clk.at = clk.at.Add(tc.after)
			want := view
			if tc.debug {
				want = debug
			}
			if _, command := commandRows(t, m); command != want {
				t.Errorf("command row = %q, want %q", command, want)
			}
		})
	}
}

// TestDebugCommandReachesTheSameCluster: the hint carries --context after
// :ctx, like every footer command.
func TestDebugCommandReachesTheSameCluster(t *testing.T) {
	t.Parallel()

	c := &k8s.Client{Context: "prod", CurrentContext: "dev", KubeconfigPaths: []string{"/home/me/.kube/config"}}
	m := New(c, nil)
	m.kubectlSession = newKubectlSession(c, sessionFlags{}, func(string) string { return "" }, "/home/me")
	done := shellDoneMsg{req: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1", Container: "app"}, shell: "sh", err: exitErr(t, "127")}
	want := "kubectl --context prod debug -it -n demo web-1 --image=busybox:1.36 --target=app"
	_, out := tuitest.Drive(t, m, selfApplyingMsg(done), tuitest.Press("y"))
	if got, _ := tuitest.Clipboard(out); got != want {
		t.Errorf("debug command = %q, want %q", got, want)
	}
}

func TestShellRunKeepsTheStderrTail(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	run := &shellRun{cmd: exec.Command("sh", "-c", "echo out; echo boom >&2; exit 3"), stderr: &tailWriter{}}
	var stdout, stderr bytes.Buffer
	run.SetStdin(strings.NewReader(""))
	run.SetStdout(&stdout)
	run.SetStderr(&stderr)
	err := run.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("Run = %v, want exit 3", err)
	}
	if stdout.String() != "out\n" || stderr.String() != "boom\n" || run.stderr.String() != "boom\n" {
		t.Errorf("stdout %q, terminal stderr %q, tail %q", stdout.String(), stderr.String(), run.stderr.String())
	}
	if run.elapsed <= 0 {
		t.Errorf("elapsed = %v", run.elapsed)
	}
}

func TestShellRunKeepsWhatItWasGiven(t *testing.T) {
	t.Parallel()

	var mine bytes.Buffer
	c := exec.Command("true")
	c.Stdin, c.Stdout, c.Stderr = strings.NewReader("x"), &mine, &mine
	run := &shellRun{cmd: c, stderr: &tailWriter{}}
	run.SetStdin(strings.NewReader(""))
	run.SetStdout(&bytes.Buffer{})
	run.SetStderr(&bytes.Buffer{})
	if c.Stdout != &mine || c.Stderr != &mine {
		t.Error("the terminal replaced streams the command already had")
	}
}

func TestTailWriterKeepsTheEnd(t *testing.T) {
	t.Parallel()

	var w tailWriter
	_, _ = w.Write(bytes.Repeat([]byte("a"), stderrTail))
	_, _ = w.Write([]byte("end"))
	got := w.String()
	if len(got) != stderrTail || !strings.HasSuffix(got, "end") {
		t.Errorf("tail is %d bytes ending %q", len(got), got[len(got)-3:])
	}
}

var _ tea.ExecCommand = (*shellRun)(nil)
