package tui

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pluginCluster is an API server that answers /version only to the token
// "good", and a kubeconfig with two contexts on it: "plain" carries the
// token, "plugin" gets it from an exec credential plugin that, like a
// login wrapper run without a terminal, fails until a login has left a
// token in its cache (a marker file here). kubectl is a stand-in that
// does that login.
type pluginCluster struct {
	kubeconfig string
	marker     string
	kubectl    string
}

func newPluginCluster(t *testing.T) pluginCluster {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the plugin and kubectl stand-ins are shell scripts")
	}
	// TLS: client-go sends no credentials at all to a plain-http server.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"Unauthorized","reason":"Unauthorized","code":401}`))
			return
		}
		_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.3+k3s1"}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	pc := pluginCluster{
		kubeconfig: filepath.Join(dir, "config"),
		marker:     filepath.Join(dir, "logged-in"),
		kubectl:    filepath.Join(dir, "kubectl"),
	}
	plugin := filepath.Join(dir, "plugin.sh")
	cred := `{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"good"}}` //nolint:gosec // the test server's fake token
	writeScript(t, plugin, fmt.Sprintf("[ -f %q ] || exit 1\nprintf '%%s\\n' '%s'\n", pc.marker, cred))
	writeScript(t, pc.kubectl, fmt.Sprintf("touch %q\n", pc.marker))
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: plain
clusters:
- name: dev
  cluster:
    server: %s
    insecure-skip-tls-verify: true
users:
- name: plain
  user:
    token: good
- name: plugin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: %s
      interactiveMode: IfAvailable
contexts:
- name: plain
  context: {cluster: dev, user: plain}
- name: plugin
  context: {cluster: dev, user: plugin}
`, srv.URL, plugin)
	if err := os.WriteFile(pc.kubeconfig, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return pc
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil { //nolint:gosec // the stand-in must be executable
		t.Fatal(err)
	}
}

// manualClock is a clock tests step by hand.
type manualClock struct{ at time.Time }

func (c *manualClock) now() time.Time { return c.at }

// TestLoginAfterContextSwitch is M4.7's acceptance: a plugin that fails
// without a terminal shows the overlay, Enter hands the terminal to
// kubectl version for that context, and k4s connects afterwards.
func TestLoginAfterContextSwitch(t *testing.T) {
	t.Parallel()

	pc := newPluginCluster(t)
	c, err := k8s.Load(k8s.Source{Paths: []string{pc.kubeconfig}, Origin: k8s.OriginFlag}, "plain")
	if err != nil {
		t.Fatal(err)
	}
	clk := &manualClock{at: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	m := New(c, nil, WithClock(clk.now))
	if loginProbe(m.client, false) != nil {
		t.Fatal("a token context was probed for a login")
	}

	next, _ := m.Update(views.ContextSelectedMsg{Name: "plugin"})
	m = next.(Model)
	probe := loginProbe(m.client, false)
	if probe == nil {
		msg, _ := m.status.Current(clk.now())
		t.Fatalf("the plugin context was not probed; context %q, status %+v", m.client.Context, msg)
	}
	failed, ok := probe().(loginProbeMsg)
	if !ok || k8s.Classify(failed.err).Kind != k8s.KindUnauthorized {
		t.Fatalf("probe = %+v, want an Unauthorized failure of the plugin", failed)
	}
	next, _ = m.Update(failed)
	m = next.(Model)
	m.width, m.height = 160, 40
	requireRoot(t, m, "log in", "Context plugin needs a login", "--context plugin version")

	m = withLookPath(m, foundAt(pc.kubectl), os.Environ())
	clk.at = clk.at.Add(overlay.Grace)
	next, cmd := m.Update(tuitest.Enter())
	m = next.(Model)
	answer := tuitest.Collect(t, cmd)
	if len(answer) != 1 {
		t.Fatalf("Enter answered %s", tuitest.Describe(answer))
	}
	next, cmd = m.Update(answer[0])
	m = next.(Model)
	if !slices.ContainsFunc(tuitest.Collect(t, cmd), tuitest.IsExec) {
		t.Fatal("Enter did not hand the terminal to kubectl")
	}

	login, err := m.loginCommand()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{pc.kubectl, "--kubeconfig", pc.kubeconfig, "--context", "plugin", "version"}
	if !slices.Equal(login.Args, want) {
		t.Errorf("login argv = %q, want %q", login.Args, want)
	}
	next, _ = m.Update(loginDoneMsg{context: "plugin", err: login.Run()})
	m = next.(Model)
	connected, _ := loginProbe(m.client, true)().(loginProbeMsg)
	if connected.err != nil {
		t.Fatalf("probe after the login: %v", connected.err)
	}
	next, _ = m.Update(connected)
	m = next.(Model)
	if msg, ok := m.status.Current(clk.now()); !ok || msg != (status.Message{Level: status.OK, Text: "logged in to plugin"}) {
		t.Errorf("status = %+v, %v", msg, ok)
	}
	if m.overlay != nil {
		t.Error("the overlay is still open")
	}
}

func requireRoot(t *testing.T, m Model, wants ...string) {
	t.Helper()
	screen := tuitest.Screen(m)
	for _, w := range wants {
		if !strings.Contains(screen, w) {
			t.Fatalf("screen lacks %q:\n%s", w, screen)
		}
	}
}

// pluginModel is a model on a fake client that claims a credential plugin.
func pluginModel(t *testing.T) Model {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithContext("eks"))
	c.RestConfig = &rest.Config{ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}
	m := New(c, nil)
	m.width, m.height = 160, 40
	return m
}

var (
	errPlugin = &url.Error{Op: "Get", URL: "https://eks/version", Err: errors.New("getting credentials: exec: executable aws failed with exit code 255")}
	errStdin  = errors.New("getting credentials: exec plugin cannot support interactive mode: standard input is unavailable")
)

func TestLoginProbeAnswers(t *testing.T) {
	t.Parallel()

	refused := &url.Error{Op: "Get", URL: "https://eks:443/version", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}
	cases := []struct {
		name        string
		msg         loginProbeMsg
		wantOverlay bool
		want        status.Message // zero: no status
	}{
		{name: "plugin failed", msg: loginProbeMsg{context: "eks", err: errPlugin}, wantOverlay: true},
		{name: "stdin unavailable", msg: loginProbeMsg{context: "eks", err: errStdin}, wantOverlay: true},
		{name: "401", msg: loginProbeMsg{context: "eks", err: apierrors.NewUnauthorized("Unauthorized")}, wantOverlay: true},
		{name: "fine", msg: loginProbeMsg{context: "eks"}},
		{name: "another failure is the views' to say", msg: loginProbeMsg{context: "eks", err: refused}},
		{name: "another context", msg: loginProbeMsg{context: "gke", err: errPlugin}},
		{
			name: "the plugin keeps no token",
			msg:  loginProbeMsg{context: "eks", afterLogin: true, err: errPlugin},
			want: status.Message{Level: status.Error, Text: "context eks: logged in, but the credential plugin still fails without a terminal: it keeps no token k4s can reuse; turn on its token cache"},
		},
		{
			name: "after the login the API is gone",
			msg:  loginProbeMsg{context: "eks", afterLogin: true, err: refused},
			want: status.Message{Level: status.Error, Text: "context eks: API unreachable: eks:443"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next, _ := pluginModel(t).Update(selfApplyingMsg(tc.msg))
			m := next.(Model)
			if got := m.overlay != nil; got != tc.wantOverlay {
				t.Errorf("overlay open = %v, want %v", got, tc.wantOverlay)
			}
			msg, ok := m.status.Current(time.Now())
			if ok != (tc.want != status.Message{}) || (ok && msg != tc.want) {
				t.Errorf("status = %+v, %v; want %+v", msg, ok, tc.want)
			}
		})
	}
}

func TestLoginProbeOverAnotherOverlay(t *testing.T) {
	t.Parallel()

	m := pluginModel(t).openHelp()
	next, _ := m.Update(selfApplyingMsg(loginProbeMsg{context: "eks", err: errPlugin}))
	m = next.(Model)
	msg, ok := m.status.Current(time.Now())
	if !ok || msg.Level != status.Warn || !strings.Contains(msg.Text, "context eks needs a login: run kubectl version in a terminal") {
		t.Errorf("status = %+v, %v", msg, ok)
	}
}

func TestLoginAnswers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		msg      selfApplyingMsg
		look     func(string) (string, error)
		wantExec bool
		want     status.Message
	}{
		{name: "yes", msg: loginAnswerMsg{context: "eks", yes: true}, look: foundAt(fakeKubectlPath), wantExec: true},
		{
			name: "yes without kubectl", msg: loginAnswerMsg{context: "eks", yes: true}, look: notFound,
			want: status.Message{Level: status.Error, Text: "login: kubectl not found on PATH"},
		},
		{
			name: "skipped", msg: loginAnswerMsg{context: "eks"}, look: foundAt(fakeKubectlPath),
			want: status.Message{Level: status.Warn, Text: "context eks needs a login: run kubectl version in a terminal, then press r"},
		},
		{name: "for another context", msg: loginAnswerMsg{context: "gke", yes: true}, look: foundAt(fakeKubectlPath)},
		{
			name: "kubectl failed", msg: loginDoneMsg{context: "eks", err: errors.New("exit status 1")}, look: foundAt(fakeKubectlPath),
			want: status.Message{Level: status.Error, Text: "login: exit status 1 · context eks needs a login: run kubectl version in a terminal, then press r"},
		},
		{
			name: "kubectl logged in", msg: loginDoneMsg{context: "eks"}, look: foundAt(fakeKubectlPath),
			want: status.Message{Level: status.Info, Text: "connecting to eks…"},
		},
		{name: "done for another context", msg: loginDoneMsg{context: "gke"}, look: foundAt(fakeKubectlPath)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := withLookPath(pluginModel(t), tc.look, nil)
			next, cmd := m.Update(tc.msg)
			if got := cmd != nil && slices.ContainsFunc(tuitest.Collect(t, cmd), tuitest.IsExec); got != tc.wantExec {
				t.Errorf("exec started = %v, want %v", got, tc.wantExec)
			}
			msg, ok := next.(Model).status.Current(time.Now())
			if ok != (tc.want != status.Message{}) || (ok && msg != tc.want) {
				t.Errorf("status = %+v, %v; want %+v", msg, ok, tc.want)
			}
		})
	}
}

// TestReloadAfterTheLogin: the view the user is on lists again.
func TestReloadAfterTheLogin(t *testing.T) {
	t.Parallel()

	m := pluginModel(t)
	m = m.replaceView(viewPods)
	next, cmd := m.Update(selfApplyingMsg(loginProbeMsg{context: "eks", afterLogin: true}))
	if next.(Model).current.Title() != viewPods || cmd == nil {
		t.Fatalf("view = %q, cmd = %v", next.(Model).current.Title(), cmd)
	}
	m = pluginModel(t)
	m.current = stubView{title: "logs demo/web-1"}
	if _, cmd := m.reloadView(); cmd != nil {
		t.Error("a view that cannot be rebuilt was reloaded")
	}
}
