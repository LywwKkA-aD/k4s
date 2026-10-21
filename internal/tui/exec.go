package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// windowsShell is what `e` runs in a Windows container, which has no sh.
	windowsShell = "cmd.exe"
	// debugImage is the image the kubectl debug hint suggests for a
	// container without a shell.
	debugImage = "busybox:1.36"

	// podLookupTimeout bounds the GET that tells a Windows pod.
	podLookupTimeout = 5 * time.Second
	// quickExit is how soon a shell that never started gives up: a 126 or
	// 127 after a real session is the last command the user typed.
	quickExit = 3 * time.Second
	// stderrTail is how much of kubectl's stderr is kept to recognise a
	// missing shell; the terminal still gets all of it.
	stderrTail = 4 << 10
)

// execDoneMsg is a step of a flow the root runs around the terminal: the
// pod lookup before `kubectl exec`, the shell that came back, and the
// login flow after :ctx (login.go). The router hands every one of them to
// onExecDone, and each applies itself to the model.
type execDoneMsg interface {
	apply(m Model) (tea.Model, tea.Cmd)
}

// onExecDone applies a step of a terminal flow.
func (m Model) onExecDone(msg execDoneMsg) (tea.Model, tea.Cmd) {
	return msg.apply(m)
}

// onExecRequest looks the pod up before handing the terminal to `kubectl
// exec -it`: a Windows pod gets cmd.exe, every other one sh, as the
// footer shows. Without a client there is nothing to look up.
func (m Model) onExecRequest(msg views.ExecRequestMsg) (tea.Model, tea.Cmd) {
	if m.client == nil {
		return m.startExec(msg, views.ExecShell)
	}
	cs := m.client.Clientset
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), podLookupTimeout)
		defer cancel()
		pod, err := cs.CoreV1().Pods(msg.Namespace).Get(ctx, msg.Pod, metav1.GetOptions{})
		return execResolvedMsg{req: msg, pod: pod, err: err}
	}
}

// execResolvedMsg is the pod an exec was asked for, or why it could not
// be read.
type execResolvedMsg struct {
	req views.ExecRequestMsg
	pod *corev1.Pod
	err error
}

// apply starts the shell the pod runs. A pod that is gone says so; any
// other failed lookup (a GET RBAC refuses while exec is allowed, a slow
// API) still tries sh, and kubectl reports what is wrong.
func (r execResolvedMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if apierrors.IsNotFound(r.err) {
		return m.setStatus(status.Warn, "exec: pod "+r.req.Namespace+"/"+r.req.Pod+" is gone, press r")
	}
	shell := views.ExecShell
	if r.err == nil {
		shell = shellFor(r.pod)
	}
	return m.startExec(r.req, shell)
}

// shellFor is the shell `e` runs in pod: cmd.exe on Windows, else sh.
// spec.os.name says so; older pods only pin the node OS.
func shellFor(pod *corev1.Pod) string {
	if pod == nil {
		return views.ExecShell
	}
	if pod.Spec.OS != nil && pod.Spec.OS.Name == corev1.Windows {
		return windowsShell
	}
	if pod.Spec.NodeSelector[corev1.LabelOSStable] == string(corev1.Windows) {
		return windowsShell
	}
	return views.ExecShell
}

// startExec hands the terminal to `kubectl exec -it ... -- <shell>`. When
// the shell exits a shellDoneMsg comes back. A missing kubectl ("kubectl
// not found on PATH") or a name kubectl would misread lands in the status
// row instead.
func (m Model) startExec(req views.ExecRequestMsg, shell string) (tea.Model, tea.Cmd) {
	c, err := m.execCommand(req, shell)
	if err != nil {
		return m.setError("exec: " + err.Error())
	}
	run := &shellRun{cmd: c, stderr: &tailWriter{}}
	return m, tea.Exec(run, func(err error) tea.Msg {
		return shellDoneMsg{req: req, shell: shell, err: err, stderr: run.stderr.String(), elapsed: run.elapsed}
	})
}

// execCommand is the exec command the footer would show for req with
// shell, decorated for this cluster, as the argv the runner starts: no
// shell in between, and KUBECONFIG pointed at the files the TUI loaded, so
// the k3s multicall kubectl cannot wander off to /etc/rancher/k3s/k3s.yaml.
func (m Model) execCommand(req views.ExecRequestMsg, shell string) (*exec.Cmd, error) {
	cmd, err := kubectl.Exec(req.Namespace, req.Pod, req.Container, shell)
	if err != nil {
		return nil, err
	}
	return m.runner.Command(m.decorate(cmd))
}

// shellDoneMsg is a kubectl exec session that ended.
type shellDoneMsg struct {
	req   views.ExecRequestMsg
	shell string
	err   error
	// stderr is the tail of what kubectl wrote to stderr; elapsed is how
	// long the session lasted.
	stderr  string
	elapsed time.Duration
}

// apply reports a failed session. A container without the shell gets
// the kubectl debug command that brings one, shown and never run.
func (d shellDoneMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if d.err == nil {
		return m, nil
	}
	if !d.missingShell() {
		return m.setError("exec: " + d.err.Error())
	}
	text := "exec: no " + d.shell + " in " + podLabel(d.req) + exitLabel(d.err)
	if debug := m.debugCommand(d); debug != "" {
		text += " · try " + debug
	}
	return m.setError(text)
}

// missingShell reports a shell that could not start: the runtime says the
// executable is not there, or kubectl came back at once with the shell's
// "cannot execute" (126) or "not found" (127).
func (d shellDoneMsg) missingShell() bool {
	if strings.Contains(d.stderr, "executable file not found") || strings.Contains(d.err.Error(), "executable file not found") {
		return true
	}
	var exit *exec.ExitError
	if !errors.As(d.err, &exit) {
		return false
	}
	code := exit.ExitCode()
	return (code == 126 || code == 127) && d.elapsed < quickExit
}

// debugCommand is `kubectl debug -it -n <ns> <pod> --image=busybox:1.36
// --target=<c>` for d, decorated like the footer; "" for a Windows pod,
// which busybox cannot serve.
func (m Model) debugCommand(d shellDoneMsg) string {
	if d.shell == windowsShell {
		return ""
	}
	args := []string{"debug", "-it", "-n", d.req.Namespace, d.req.Pod, "--image=" + debugImage}
	if d.req.Container != "" {
		args = append(args, "--target="+d.req.Container)
	}
	return m.decorate(kubectl.New(args...)).String()
}

func podLabel(req views.ExecRequestMsg) string {
	if req.Container == "" {
		return req.Pod
	}
	return req.Pod + "/" + req.Container
}

func exitLabel(err error) string {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return ""
	}
	return fmt.Sprintf(" (exit %d)", exit.ExitCode())
}

// shellRun is the kubectl exec process handed to the terminal. It keeps
// the tail of kubectl's stderr, which still reaches the terminal, and how
// long the session lasted, so a missing shell can be told from a session
// that ended badly.
type shellRun struct {
	cmd     *exec.Cmd
	stderr  *tailWriter
	elapsed time.Duration
}

// Run runs the process; the callback reads elapsed once it returns.
func (s *shellRun) Run() error {
	start := time.Now()
	err := s.cmd.Run()
	s.elapsed = time.Since(start)
	return err
}

// SetStdin gives the process the terminal's input.
func (s *shellRun) SetStdin(r io.Reader) {
	if s.cmd.Stdin == nil {
		s.cmd.Stdin = r
	}
}

// SetStdout gives the process the terminal's output.
func (s *shellRun) SetStdout(w io.Writer) {
	if s.cmd.Stdout == nil {
		s.cmd.Stdout = w
	}
}

// SetStderr gives the process the terminal's stderr, copied into the tail.
func (s *shellRun) SetStderr(w io.Writer) {
	if s.cmd.Stderr == nil {
		s.cmd.Stderr = io.MultiWriter(w, s.stderr)
	}
}

// tailWriter keeps the last stderrTail bytes written to it.
type tailWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	buf := append(w.buf, p...)
	if over := len(buf) - stderrTail; over > 0 {
		buf = slices.Clone(buf[over:])
	}
	w.buf = buf
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

func kubeconfigPaths(c *k8s.Client) []string {
	if c == nil {
		return nil
	}
	return c.KubeconfigPaths
}
