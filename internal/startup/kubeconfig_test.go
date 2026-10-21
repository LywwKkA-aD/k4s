package startup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const kubeconfigBody = `apiVersion: v1
kind: Config
current-context: alpha
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: alpha
  context: {cluster: c1, user: u1, namespace: team-a}
- name: beta
  context: {cluster: c1, user: u1}
- name: k3d-dev
  context: {cluster: c1, user: u1}
users:
- name: u1
  user: {token: not-a-real-token}
`

// machine is a fake home directory and k3s server for one test.
type machine struct {
	home string
	k3s  string
	env  map[string]string
}

func newMachine(t *testing.T) machine {
	t.Helper()
	root := t.TempDir()
	return machine{
		home: filepath.Join(root, "home"),
		k3s:  filepath.Join(root, "etc", "rancher", "k3s", "k3s.yaml"),
		env:  map[string]string{},
	}
}

func (m machine) Env() Env {
	return Env{Getenv: func(k string) string { return m.env[k] }, Home: m.home, K3sKubeconfig: m.k3s}
}

func (m machine) homeConfig() string { return filepath.Join(m.home, ".kube", "config") }

func write(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// unreadable writes a kubeconfig that the current user may not open.
func unreadable(t *testing.T, path string) string {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not stop this user")
	}
	write(t, path, kubeconfigBody)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	return path
}

func TestResolveSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		explicit func(m machine) string
		setup    func(t *testing.T, m machine)
		want     func(m machine) k8s.Source
		wantErr  error
	}{
		{
			name:  "home config",
			setup: func(t *testing.T, m machine) { write(t, m.homeConfig(), kubeconfigBody) },
			want:  func(m machine) k8s.Source { return k8s.Source{Paths: []string{m.homeConfig()}} },
		},
		{
			name: "home config wins over k3s",
			setup: func(t *testing.T, m machine) {
				write(t, m.homeConfig(), kubeconfigBody)
				write(t, m.k3s, kubeconfigBody)
			},
			want: func(m machine) k8s.Source { return k8s.Source{Paths: []string{m.homeConfig()}} },
		},
		{
			name:  "k3s fallback",
			setup: func(t *testing.T, m machine) { write(t, m.k3s, kubeconfigBody) },
			want: func(m machine) k8s.Source {
				return k8s.Source{Paths: []string{m.k3s}, Origin: k8s.OriginK3s}
			},
		},
		{
			name: "KUBECONFIG wins over home, keeps order, drops blanks and repeats",
			setup: func(t *testing.T, m machine) {
				write(t, m.homeConfig(), kubeconfigBody)
				a := write(t, filepath.Join(m.home, "a"), kubeconfigBody)
				m.env["KUBECONFIG"] = strings.Join([]string{a, "", filepath.Join(m.home, "missing"), a}, string(os.PathListSeparator))
			},
			want: func(m machine) k8s.Source {
				return k8s.Source{Paths: []string{filepath.Join(m.home, "a"), filepath.Join(m.home, "missing")}}
			},
		},
		{
			name: "flag wins over KUBECONFIG",
			explicit: func(m machine) string {
				return filepath.Join(m.home, "flag")
			},
			setup: func(t *testing.T, m machine) {
				write(t, filepath.Join(m.home, "flag"), kubeconfigBody)
				m.env["KUBECONFIG"] = write(t, filepath.Join(m.home, "a"), kubeconfigBody)
			},
			want: func(m machine) k8s.Source {
				return k8s.Source{Paths: []string{filepath.Join(m.home, "flag")}, Origin: k8s.OriginFlag}
			},
		},
		{
			name:     "missing flag file",
			explicit: func(m machine) string { return filepath.Join(m.home, "flag") },
			setup:    func(*testing.T, machine) {},
			wantErr:  fs.ErrNotExist,
		},
		{
			name:    "nothing to load",
			setup:   func(*testing.T, machine) {},
			wantErr: errNoKubeconfig,
		},
		{
			name: "KUBECONFIG files all missing",
			setup: func(_ *testing.T, m machine) {
				m.env["KUBECONFIG"] = filepath.Join(m.home, "missing")
			},
			wantErr: errNoKubeconfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			tc.setup(t, m)
			explicit := ""
			if tc.explicit != nil {
				explicit = tc.explicit(m)
			}
			got, err := resolveSource(explicit, m.Env())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("resolveSource err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSource: %v", err)
			}
			want := tc.want(m)
			if got.Origin != want.Origin || !slices.Equal(got.Paths, want.Paths) {
				t.Fatalf("resolveSource = %+v, want %+v", got, want)
			}
		})
	}
}

func TestResolveSourcePermissionFix(t *testing.T) {
	t.Parallel()

	const copyFix = "mkdir -p ~/.kube && sudo k3s kubectl config view --raw > ~/.kube/config && chmod 600 ~/.kube/config"
	cases := []struct {
		name     string
		setup    func(t *testing.T, m machine) (explicit string)
		wantFix  []string
		wantNone []string
	}{
		{
			name: "k3s fallback",
			setup: func(t *testing.T, m machine) string {
				unreadable(t, m.k3s)
				return ""
			},
			wantFix:  []string{copyFix, "--write-kubeconfig-mode"},
			wantNone: []string{"KUBECONFIG", "--kubeconfig"},
		},
		{
			name: "KUBECONFIG names the k3s file",
			setup: func(t *testing.T, m machine) string {
				m.env["KUBECONFIG"] = unreadable(t, m.k3s)
				return ""
			},
			wantFix: []string{copyFix, "then unset KUBECONFIG"},
		},
		{
			name: "flag names the k3s file",
			setup: func(t *testing.T, m machine) string {
				return unreadable(t, m.k3s)
			},
			wantFix: []string{copyFix, "then start k4s without --kubeconfig"},
		},
		{
			name: "another unreadable file gets no k3s advice",
			setup: func(t *testing.T, m machine) string {
				unreadable(t, m.homeConfig())
				return ""
			},
			wantNone: []string{"sudo", "k3s"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMachine(t)
			explicit := tc.setup(t, m)
			_, err := resolveSource(explicit, m.Env())
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("resolveSource err = %v, want a permission error", err)
			}
			var re *readError
			if !errors.As(err, &re) {
				t.Fatalf("resolveSource err = %T, want *readError", err)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "open ") || !strings.Contains(msg, "permission denied") {
				t.Errorf("first line should be the open error:\n%s", msg)
			}
			for _, s := range tc.wantFix {
				if !strings.Contains(msg, s) {
					t.Errorf("message lacks %q:\n%s", s, msg)
				}
			}
			// The first line holds a temp path named after the test.
			_, fix, _ := strings.Cut(msg, "\n")
			for _, s := range tc.wantNone {
				if strings.Contains(fix, s) {
					t.Errorf("advice should not mention %q:\n%s", s, msg)
				}
			}
		})
	}
}
