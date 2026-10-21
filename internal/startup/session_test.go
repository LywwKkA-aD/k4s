package startup

import (
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		opts          cli.Options
		wantContext   string
		wantNamespace string
		wantDistro    cluster.Distro
	}{
		{name: "context namespace", wantContext: "alpha", wantNamespace: "team-a", wantDistro: cluster.Other},
		{name: "context without a namespace starts in all", opts: cli.Options{Context: "beta"}, wantContext: "beta", wantNamespace: ""},
		{name: "-n overrides the context", opts: cli.Options{Namespace: "web"}, wantContext: "alpha", wantNamespace: "web"},
		{name: "-A forces all", opts: cli.Options{AllNamespaces: true}, wantContext: "alpha", wantNamespace: ""},
		{name: "-A wins over -n", opts: cli.Options{AllNamespaces: true, Namespace: "web"}, wantContext: "alpha", wantNamespace: ""},
		{name: "distro from the context name", opts: cli.Options{Context: "k3d-dev"}, wantContext: "k3d-dev", wantDistro: cluster.K3d},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			write(t, m.homeConfig(), kubeconfigBody)
			s, err := Open(tc.opts, m.Env())
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if s.LoadErr != nil || s.Client == nil {
				t.Fatalf("Open = %+v, want a client", s)
			}
			if s.Client.Context != tc.wantContext || s.Client.Namespace != tc.wantNamespace {
				t.Errorf("context/namespace = %q/%q, want %q/%q", s.Client.Context, s.Client.Namespace, tc.wantContext, tc.wantNamespace)
			}
			if tc.wantDistro != "" && s.Distro != tc.wantDistro {
				t.Errorf("Distro = %q, want %q", s.Distro, tc.wantDistro)
			}
			if s.Source.Paths[0] != m.homeConfig() || s.Client.Source.Paths[0] != m.homeConfig() {
				t.Errorf("Source = %+v / %+v, want %s", s.Source, s.Client.Source, m.homeConfig())
			}
		})
	}
}

func TestOpenK3sFallbackRecordsThePath(t *testing.T) {
	t.Parallel()

	m := newMachine(t)
	write(t, m.k3s, kubeconfigBody)
	s, err := Open(cli.Options{}, m.Env())
	if err != nil || s.Client == nil {
		t.Fatalf("Open = %+v, %v", s, err)
	}
	if got := s.Client.KubeconfigPaths; len(got) != 1 || got[0] != m.k3s {
		t.Errorf("KubeconfigPaths = %v, want [%s]", got, m.k3s)
	}
	if s.Client.KubeconfigArg() != m.k3s {
		t.Errorf("KubeconfigArg() = %q, want %s", s.Client.KubeconfigArg(), m.k3s)
	}
}

func TestOpenFailures(t *testing.T) {
	t.Parallel()

	const badYAML = "apiVersion: v1\nkind: Config\nclusters: {{{\n"
	cases := []struct {
		name      string
		setup     func(t *testing.T, m machine) cli.Options
		fatal     bool
		wantErr   string
		wantIsErr error
	}{
		{
			name: "unknown --context exits",
			setup: func(t *testing.T, m machine) cli.Options {
				write(t, m.homeConfig(), kubeconfigBody)
				return cli.Options{Context: "gamma"}
			},
			fatal: true, wantErr: `context "gamma" not found in kubeconfig`,
		},
		{
			name: "missing --kubeconfig exits",
			setup: func(_ *testing.T, m machine) cli.Options {
				return cli.Options{Kubeconfig: filepath.Join(m.home, "nope")}
			},
			fatal: true, wantIsErr: fs.ErrNotExist,
		},
		{
			name: "bad --kubeconfig exits",
			setup: func(t *testing.T, m machine) cli.Options {
				return cli.Options{Kubeconfig: write(t, filepath.Join(m.home, "bad"), badYAML)}
			},
			fatal: true, wantErr: "load kubeconfig: ",
		},
		{
			name: "unreadable k3s kubeconfig exits with the fix",
			setup: func(t *testing.T, m machine) cli.Options {
				unreadable(t, m.k3s)
				return cli.Options{}
			},
			fatal: true, wantIsErr: fs.ErrPermission,
		},
		{
			name:    "no kubeconfig opens the tui with the reason",
			setup:   func(*testing.T, machine) cli.Options { return cli.Options{} },
			wantErr: "no kubeconfig found: pass --kubeconfig, set KUBECONFIG or create ~/.kube/config",
		},
		{
			name: "no kubeconfig with --context exits",
			setup: func(*testing.T, machine) cli.Options {
				return cli.Options{Context: "alpha"}
			},
			fatal: true, wantIsErr: errNoKubeconfig,
		},
		{
			name: "bad default kubeconfig opens the tui with the reason",
			setup: func(t *testing.T, m machine) cli.Options {
				write(t, m.homeConfig(), badYAML)
				return cli.Options{}
			},
			wantErr: "load kubeconfig: ",
		},
		{
			name: "missing current-context opens the tui with the reason",
			setup: func(t *testing.T, m machine) cli.Options {
				write(t, m.homeConfig(), strings.Replace(kubeconfigBody, "current-context: alpha", "", 1))
				return cli.Options{}
			},
			wantErr: "kubeconfig has no current-context",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			opts := tc.setup(t, m)
			s, err := Open(opts, m.Env())
			got := s.LoadErr
			if tc.fatal {
				got = err
				if s.Client != nil || s.LoadErr != nil {
					t.Errorf("fatal Open returned a session: %+v", s)
				}
			} else if err != nil || s.Client != nil {
				t.Fatalf("Open = %+v, %v; want a session without a client", s, err)
			}
			if got == nil {
				t.Fatal("no error")
			}
			if tc.wantErr != "" && !strings.Contains(got.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", got, tc.wantErr)
			}
			if tc.wantIsErr != nil && !errors.Is(got, tc.wantIsErr) {
				t.Errorf("error = %v, want errors.Is %v", got, tc.wantIsErr)
			}
		})
	}
}

func TestOpenKeepsTheSourceWhenLoadingFails(t *testing.T) {
	t.Parallel()

	m := newMachine(t)
	write(t, m.homeConfig(), "apiVersion: v1\nkind: Config\nclusters: {{{\n")
	s, err := Open(cli.Options{}, m.Env())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.Source.Origin != k8s.OriginKubectl || len(s.Source.Paths) != 1 || s.Source.Paths[0] != m.homeConfig() {
		t.Fatalf("Source = %+v, want the home config", s.Source)
	}
}

func TestSystemEnv(t *testing.T) {
	t.Parallel()

	env := SystemEnv()
	if env.Getenv == nil {
		t.Fatal("SystemEnv has no Getenv")
	}
	wantK3s := ""
	if runtime.GOOS == "linux" {
		wantK3s = "/etc/rancher/k3s/k3s.yaml"
	}
	if env.K3sKubeconfig != wantK3s {
		t.Errorf("K3sKubeconfig = %q, want %q", env.K3sKubeconfig, wantK3s)
	}
}
