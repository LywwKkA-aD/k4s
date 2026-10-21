package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistroName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		d    Distro
		want string
	}{
		{d: K3s, want: "k3s"},
		{d: K3d, want: "k3d"},
		{d: RKE2, want: "rke2"},
		{d: EKS, want: "eks"},
		{d: GKE, want: "gke"},
		{d: AKS, want: "aks"},
		{d: Kind, want: "kind"},
		{d: Minikube, want: "minikube"},
		{d: Other, want: "k8s"},
		{d: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.d)+"_", func(t *testing.T) {
			t.Parallel()
			if got := tc.d.Name(); got != tc.want {
				t.Fatalf("%q.Name() = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestRelease(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "v1.36.5+k3s1", want: "v1.36.5"},
		{in: "v1.30.4+rke2r1", want: "v1.30.4"},
		{in: "v1.30.4-eks-a737599", want: "v1.30.4"},
		{in: "v1.30.5-gke.1014001", want: "v1.30.5"},
		{in: "v1.31.0", want: "v1.31.0"},
		{in: "v1.31", want: ""},
		{in: "1.31.0", want: ""},
		{in: "", want: ""},
		{in: "v1.36.5\x1b]52;c;ZXZpbA==\x07", want: "v1.36.5"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := Release(tc.in); got != tc.want {
				t.Fatalf("Release(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		server string
		want   string
	}{
		{name: "k3s default port is dropped", server: "https://10.0.0.5:6443", want: "10.0.0.5"},
		{name: "https default port is dropped", server: "https://api.example.com:443", want: "api.example.com"},
		{name: "no port", server: "https://api.example.com", want: "api.example.com"},
		{name: "other ports are kept", server: "https://127.0.0.1:7443", want: "127.0.0.1:7443"},
		{name: "ipv6 with a port", server: "https://[fd00::5]:7443", want: "[fd00::5]:7443"},
		{name: "ipv6 on the k3s port", server: "https://[fd00::5]:6443", want: "fd00::5"},
		{name: "no scheme", server: "10.0.0.6:6443", want: "10.0.0.6"},
		{name: "user info never shows", server: "https://deploy@10.0.0.7:6443/k8s", want: "10.0.0.7"},
		{name: "empty", server: "", want: ""},
		{name: "unparsable", server: "https://%zz", want: ""},
		{name: "control characters are dropped", server: "https://evil\x1b[2J.example:6443", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Host(tc.server)
			if got != tc.want {
				t.Fatalf("Host(%q) = %q, want %q", tc.server, got, tc.want)
			}
			if strings.ContainsAny(got, "\x1b\x07") {
				t.Fatalf("Host(%q) = %q carries a control character", tc.server, got)
			}
		})
	}
}

// writeKubeconfig writes a kubeconfig defining contexts names into dir/file.
func writeKubeconfig(t *testing.T, dir, file string, names ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Config\ncontexts:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: c\n    user: u\n", n)
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// symlink makes link point at target, skipping the test where the system
// does not allow it.
func symlink(t *testing.T, target, link string) string {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	return link
}

func TestAmbiguous(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := writeKubeconfig(t, dir, "a", "prod", "lab")
	b := writeKubeconfig(t, dir, "b", "prod", "edge")
	broken := filepath.Join(dir, "broken")
	if err := os.WriteFile(broken, []byte("{not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	toA, toB := symlink(t, a, filepath.Join(dir, "to-a")), symlink(t, b, filepath.Join(dir, "to-b"))

	cases := []struct {
		name    string
		context string
		files   []string
		want    bool
	}{
		{name: "every stock k3s context is called default", context: "default", want: true},
		{name: "default in one file", context: "default", files: []string{a}, want: true},
		{name: "a name two files define", context: "prod", files: []string{a, b}, want: true},
		{name: "a name one file defines", context: "lab", files: []string{a, b}, want: false},
		{name: "the same file listed twice", context: "lab", files: []string{a, a}, want: false},
		{name: "the same file spelled two ways", context: "lab", files: []string{a, dir + "/./a"}, want: false},
		{name: "the same file through a symlink", context: "lab", files: []string{a, toA}, want: false},
		{name: "a symlink to another file", context: "prod", files: []string{a, toB}, want: true},
		{name: "missing and broken files are skipped", context: "prod", files: []string{missing, broken, a}, want: false},
		{name: "no files", context: "prod", want: false},
		{name: "no context", context: "", files: []string{a, b}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Ambiguous(tc.context, tc.files); got != tc.want {
				t.Fatalf("Ambiguous(%q, %v) = %v, want %v", tc.context, tc.files, got, tc.want)
			}
		})
	}
}

func TestMetricsHint(t *testing.T) {
	t.Parallel()

	bundled := "metrics-server is bundled with k3s; disabled with --disable=metrics-server?"
	install := "install metrics-server: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml"
	cases := []struct {
		d    Distro
		want string
	}{
		{d: K3s, want: bundled},
		{d: K3d, want: bundled},
		{d: RKE2, want: "metrics-server is bundled with rke2; disabled with --disable=rke2-metrics-server?"},
		{d: Minikube, want: "enable metrics-server: minikube addons enable metrics-server"},
		{d: Kind, want: install},
		{d: EKS, want: install},
		{d: GKE, want: install},
		{d: AKS, want: install},
		{d: Other, want: install},
		{d: "", want: install},
	}
	for _, tc := range cases {
		t.Run(string(tc.d)+"_", func(t *testing.T) {
			t.Parallel()
			if got := MetricsHint(tc.d); got != tc.want {
				t.Fatalf("MetricsHint(%q) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}
