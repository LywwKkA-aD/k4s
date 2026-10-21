package tui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// clusterAt is a fake client for context name on the API server at server.
func clusterAt(t *testing.T, name, server string) *k8s.Client {
	t.Helper()
	c := *tuitest.NewFakeClient(t, tuitest.WithContext(name))
	c.RestConfig = &rest.Config{Host: server}
	return &c
}

// knownVersion is the session startup hands over once the server answered.
func knownVersion(d cluster.Distro, gitVersion string) startup.Session {
	return startup.Session{Distro: d, ServerVersion: &apiversion.Info{GitVersion: gitVersion}}
}

// headerAt mounts the root at width columns and returns the header line.
func headerAt(t *testing.T, c *k8s.Client, s startup.Session, width int) string {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(s)), width, 24)
	return header(m)
}

type headerCase struct {
	name       string
	distro     cluster.Distro
	context    string
	server     string
	gitVersion string
	namespace  string
}

// distributions are one realistic session per distribution k4s tells
// apart: stock kubeconfig context names, servers and versions.
var distributions = []headerCase{
	{name: "k3s", distro: cluster.K3s, context: "default", server: "https://10.0.0.5:6443", gitVersion: "v1.36.5+k3s1"},
	{name: "k3d", distro: cluster.K3d, context: "k3d-dev", server: "https://0.0.0.0:41235", gitVersion: "v1.31.4+k3s1"},
	{name: "rke2", distro: cluster.RKE2, context: "default", server: "https://192.168.1.20:6443", gitVersion: "v1.30.4+rke2r1", namespace: "kube-system"},
	{name: "eks", distro: cluster.EKS, context: "arn:aws:eks:eu-west-1:123456789012:cluster/payments-production", server: "https://0A1B2C3D4E5F.gr7.eu-west-1.eks.amazonaws.com", gitVersion: "v1.30.4-eks-a737599", namespace: "payments"},
	{name: "gke", distro: cluster.GKE, context: "gke_acme-prod_europe-west1_payments", server: "https://34.76.10.20", gitVersion: "v1.30.5-gke.1014001"},
	{name: "aks", distro: cluster.AKS, context: "payments-aks", server: "https://payments-dns-1a2b3c.hcp.westeurope.azmk8s.io:443", gitVersion: "v1.30.5"},
	{name: "kind", distro: cluster.Kind, context: "kind-k4s", server: "https://127.0.0.1:39051", gitVersion: "v1.33.1"},
	{name: "minikube", distro: cluster.Minikube, context: "minikube", server: "https://192.168.49.2:8443", gitVersion: "v1.33.1"},
	{name: "vanilla", distro: cluster.Other, context: "kubernetes-admin@kubernetes", server: "https://10.1.0.10:6443", gitVersion: "v1.31.0"},
}

func (hc headerCase) client(t *testing.T) *k8s.Client {
	t.Helper()
	return clusterAt(t, hc.context, hc.server).WithNamespace(hc.namespace)
}

// TestHeaderGoldens pins the header of every distribution at 80 and 160
// columns. Regenerate with
//
//	go test ./internal/tui -run TestHeaderGoldens -update
func TestHeaderGoldens(t *testing.T) {
	t.Parallel()

	for _, width := range []int{80, 160} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			for _, hc := range distributions {
				h := headerAt(t, hc.client(t), knownVersion(hc.distro, hc.gitVersion), width)
				if w := ansi.StringWidth(h); w > width {
					t.Errorf("%s: header is %d cells wide on %d columns: %q", hc.name, w, width, h)
				}
				fmt.Fprintf(&b, "%-8s| %s\n", hc.name, h)
			}
			tuitest.Golden(t, fmt.Sprintf("header_%d", width), b.String())
		})
	}
}

func TestHeaderBeforeTheVersionIsKnown(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		context string
		session startup.Session
		want    string
	}{
		{name: "a distribution from the context name alone", context: "k3d-dev", session: startup.Session{Distro: cluster.K3d}, want: "k4s · k3d · ctx k3d-dev · ns ALL · dashboard"},
		{name: "nothing known yet", context: "lab", session: startup.Session{Distro: cluster.Other}, want: "k4s · ctx lab · ns ALL · dashboard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := headerAt(t, clusterAt(t, tc.context, "https://10.0.0.9:6443"), tc.session, 160); got != tc.want {
				t.Fatalf("header = %q, want %q", got, tc.want)
			}
		})
	}
}

// Every stock k3s kubeconfig names its context default; the host is what
// tells two of them apart.
func TestTwoDefaultContextsShowDifferentHosts(t *testing.T) {
	t.Parallel()

	session := knownVersion(cluster.K3s, "v1.36.5+k3s1")
	first := headerAt(t, clusterAt(t, "default", "https://10.0.0.5:6443"), session, 80)
	second := headerAt(t, clusterAt(t, "default", "https://10.0.0.6:6443"), session, 80)
	if first == second {
		t.Fatalf("both clusters show the same header %q", first)
	}
	for h, want := range map[string]string{first: "ctx default (10.0.0.5)", second: "ctx default (10.0.0.6)"} {
		if !strings.Contains(h, want) {
			t.Errorf("header %q lacks %q", h, want)
		}
	}
}

func TestHeaderShowsTheHostOnlyForAmbiguousNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, err := tuitest.WriteKubeconfig(dir, "prod", "prod", "lab")
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := tuitest.WriteKubeconfig(other, "prod", "prod")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		context string
		want    string
		not     string
	}{
		{name: "a name two kubeconfig files define", context: "prod", want: "ctx prod (10.0.0.7)"},
		{name: "a name only one file defines", context: "lab", want: "ctx lab ·", not: "10.0.0.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := clusterAt(t, tc.context, "https://10.0.0.7:6443")
			c.KubeconfigPaths = []string{a, b}
			h := headerAt(t, c, knownVersion(cluster.K3s, "v1.36.5+k3s1"), 160)
			if !strings.Contains(h, tc.want) {
				t.Errorf("header %q lacks %q", h, tc.want)
			}
			if tc.not != "" && strings.Contains(h, tc.not) {
				t.Errorf("header %q shows %q", h, tc.not)
			}
		})
	}
}

// TestHeaderTruncationOrder narrows one long header step by step: the
// version goes first, then the distribution, then the context name is
// shortened in the middle, then the host goes, and last the line is cut at
// its end. Regenerate with
//
//	go test ./internal/tui -run TestHeaderTruncationOrder -update
func TestHeaderTruncationOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "arn:aws:eks:eu-west-1:123456789012:cluster/payments"
	a, err := tuitest.WriteKubeconfig(dir, name, name)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := tuitest.WriteKubeconfig(other, name, name)
	if err != nil {
		t.Fatal(err)
	}
	c := clusterAt(t, name, "https://10.20.30.40:6443").WithNamespace("payments")
	c.KubeconfigPaths = []string{a, b}
	session := knownVersion(cluster.EKS, "v1.30.4-eks-a737599")

	var out strings.Builder
	for _, width := range []int{120, 116, 110, 100, 70, 60, 40, 24, 12} {
		h := headerAt(t, c, session, width)
		if w := ansi.StringWidth(h); w > width {
			t.Errorf("header is %d cells wide on %d columns: %q", w, width, h)
		}
		fmt.Fprintf(&out, "%3d| %s\n", width, h)
	}
	tuitest.Golden(t, "header_truncation", out.String())
}

func TestWindowTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		client func(t *testing.T) *k8s.Client
		want   string
	}{
		{name: "all namespaces", client: func(t *testing.T) *k8s.Client { return clusterAt(t, "default", "https://10.0.0.5:6443") }, want: "k4s · default/ALL"},
		{name: "one namespace", client: func(t *testing.T) *k8s.Client {
			return clusterAt(t, "lab", "https://10.0.0.5:6443").WithNamespace("demo")
		}, want: "k4s · lab/demo"},
		{name: "control characters never reach the terminal", client: func(t *testing.T) *k8s.Client {
			return clusterAt(t, "lab\x07\x1b]0;evil", "https://10.0.0.5:6443")
		}, want: "k4s · lab/ALL"},
		{name: "no client", client: func(*testing.T) *k8s.Client { return nil }, want: "k4s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(tc.client(t), nil), 80, 24)
			if got := m.View().WindowTitle; got != tc.want {
				t.Fatalf("WindowTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

// versionedClient answers /version from memory with gitVersion.
func versionedClient(name, gitVersion string) *k8s.Client {
	cs := fake.NewSimpleClientset()
	if fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery); ok {
		fd.FakedServerVersion = &apiversion.Info{GitVersion: gitVersion}
	}
	return &k8s.Client{Clientset: cs, Context: name, CurrentContext: name}
}

// Without a preflight (every cert or token kubeconfig) the header asks the
// server for its version once the TUI is up.
func TestHeaderFetchesTheServerVersion(t *testing.T) {
	t.Parallel()

	c := versionedClient("default", "v1.36.5+k3s1")
	m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(startup.Session{Distro: cluster.Other})), 120, 24)
	if h, want := header(m), "k4s · k3s v1.36.5 · ctx default · ns ALL · dashboard"; h != want {
		t.Fatalf("header = %q, want %q", h, want)
	}
}

func TestHeaderMarkupNeverWraps(t *testing.T) {
	t.Parallel()

	c := clusterAt(t, "default", "https://10.0.0.5:6443")
	for _, width := range []int{30, 50, 80} {
		m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(knownVersion(cluster.K3s, "v1.36.5+k3s1"))), width, 24)
		ls := lines(m)
		if !isRule(ls[1]) {
			t.Errorf("%d columns: the header takes more than one line:\n%s", width, tuitest.Screen(m))
		}
	}
}
