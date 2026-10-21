package tui_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// startedWith is started with options.
func startedWith(t *testing.T, c *k8s.Client, loadErr error, opts ...tui.Option) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, loadErr, opts...), screenW, screenH)
	return m
}

func TestStartsInTheClientNamespace(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t).WithNamespace(tuitest.NamespaceDemo)
	m, _ := tuitest.Drive(t, startedWith(t, c, nil), colon("pods")...)
	if !strings.Contains(header(m), "ns: "+tuitest.NamespaceDemo) {
		t.Errorf("header = %q", header(m))
	}
	if got := commandRow(m); got != "kubectl get pods -n demo" {
		t.Errorf("command row = %q", got)
	}
	refuteScreen(t, m, tuitest.PodCoreDNS)
}

// TestContextFlagStaysOnFooterCommands: a context named on the command
// line is named in every pasted command too, even when it is the
// kubeconfig's current one.
func TestContextFlagStaysOnFooterCommands(t *testing.T) {
	t.Parallel()

	m := startedWith(t, tuitest.NewFakeClient(t), nil, tui.WithFlags(cli.Options{Context: tuitest.FakeContext}))
	if got := commandRow(m); got != "kubectl --context fake get all -A" {
		t.Errorf("command row = %q", got)
	}
	plain := started(t, tuitest.NewFakeClient(t))
	if got := commandRow(plain); got != "kubectl get all -A" {
		t.Errorf("without --context the command row = %q", got)
	}
}

func TestPreflightErrorShowsAtStart(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "bare error", err: errors.New("get server version: Unauthorized"), want: "preflight: get server version: Unauthorized"},
		{name: "already prefixed", err: errors.New("preflight: no answer from 10.0.0.5:6443 within 30s"), want: "preflight: no answer from 10.0.0.5:6443 within 30s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t)
			m := startedWith(t, c, nil, tui.WithSession(startup.Session{Client: c, PreflightErr: tc.err}))
			if got := footer(m); !strings.HasPrefix(got, " "+tc.want+"\n") {
				t.Errorf("footer = %q, want the status %q", got, tc.want)
			}
		})
	}
}

// sessionKubeconfig writes a kubeconfig with the contexts gamma (current)
// and delta, and returns it as a --kubeconfig Source.
func sessionKubeconfig(t *testing.T) k8s.Source {
	t.Helper()
	path, err := tuitest.WriteKubeconfig(t.TempDir(), "gamma", "gamma", "delta")
	if err != nil {
		t.Fatal(err)
	}
	return k8s.Source{Paths: []string{path}, Origin: k8s.OriginFlag}
}

// TestContextSwitchReadsTheSessionKubeconfig: without a client (the load
// failed) the picker and the switch still read the files startup chose,
// not KUBECONFIG, and pasted commands name that file.
func TestContextSwitchReadsTheSessionKubeconfig(t *testing.T) {
	t.Parallel()

	src := sessionKubeconfig(t)
	loadErr := errors.New("load kubeconfig: no current-context")
	m := startedWith(t, nil, loadErr,
		tui.WithSession(startup.Session{Source: src, LoadErr: loadErr}),
		tui.WithFlags(cli.Options{Kubeconfig: src.Paths[0]}))
	// Wide enough for the temporary kubeconfig path in the command row.
	m, _ = tuitest.Drive(t, m, tuitest.Resize(400, screenH))
	m, _ = tuitest.Drive(t, m, colon("ctx")...)
	requireScreen(t, m, "gamma-cluster", "delta-cluster")
	refuteScreen(t, m, "alpha-cluster")

	// Rows sort by name: delta comes first.
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	requireView(t, m, "dashboard")
	h := header(m)
	if !strings.Contains(h, "ctx: delta") || !strings.Contains(h, "ns: "+tuitest.NamespaceDemo) {
		t.Errorf("header after the switch = %q", h)
	}
	want := fmt.Sprintf("kubectl --kubeconfig %s --context delta get all -A", src.Paths[0])
	if got := commandRow(m); got != want {
		t.Errorf("command row = %q\nwant          %q", got, want)
	}

	// Switching again goes through the new client and keeps the file;
	// gamma is the file's current-context, so it needs no --context.
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "gamma"})
	want = fmt.Sprintf("kubectl --kubeconfig %s get all -A", src.Paths[0])
	if got := commandRow(m); got != want {
		t.Errorf("command row after the second switch = %q", got)
	}
}

// TestFooterAfterContextSwitch pins every view's kubectl row after :ctx to
// a context that is not the kubeconfig's current one: a pasted command
// must reach the cluster k4s shows. Only `kubectl config` commands, which
// work on the file itself, go without --context.
func TestFooterAfterContextSwitch(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name  string
		input []tea.Msg
	}{
		{name: "dashboard"},
		{name: "pods", input: colon("pods")},
		{name: "deployments", input: colon("deploy")},
		{name: "services", input: colon("svc")},
		{name: "namespaces", input: colon("ns")},
		{name: "top pods", input: colon("top")},
		{name: "top nodes", input: then(colon("top"), tuitest.Keys("n"))},
		{name: "describe", input: []tea.Msg{views.DescribeRequestMsg{Kind: "pod", Namespace: demo, Name: tuitest.PodWeb1}}},
		{name: "logs", input: []tea.Msg{views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}, Tail: 100}}},
		{name: "tail prompt", input: []tea.Msg{views.TailPromptRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}}}},
		{name: "contexts", input: colon("ctx")},
	}
	rows := make([]string, len(cases))
	for i, tc := range cases {
		m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.ContextSelectedMsg{Name: "other"})
		m, _ = tuitest.Drive(t, m, tc.input...)
		row := commandRow(m)
		if !strings.Contains(row, "--context other ") && !strings.HasPrefix(row, "kubectl config ") {
			t.Errorf("%s: command row %q lacks --context other", tc.name, row)
		}
		rows[i] = tc.name + ": " + row
	}
	tuitest.Golden(t, "footer_after_ctx_other", strings.Join(rows, "\n")+"\n")
}

// TestK3sFallbackFooter: k4s read the k3s server's kubeconfig, which a
// bare kubectl never does, so every pasted command names it.
func TestK3sFallbackFooter(t *testing.T) {
	t.Parallel()

	const k3sYAML = "/etc/rancher/k3s/k3s.yaml"
	c := tuitest.NewFakeClient(t)
	c.Source = k8s.Source{Paths: []string{k3sYAML}, Origin: k8s.OriginK3s}
	c.KubeconfigPaths = []string{k3sYAML}
	m := startedWith(t, c, nil, tui.WithSession(startup.Session{Client: c, Source: c.Source}))
	m, _ = tuitest.Drive(t, m, colon("pods")...)
	tuitest.Golden(t, "footer_k3s_fallback", footer(m)+"\n")
	if got := commandRow(m); got != "kubectl --kubeconfig "+k3sYAML+" get pods -A" {
		t.Errorf("command row = %q", got)
	}
}
