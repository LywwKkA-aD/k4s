package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const fakeToken = "s3cr3t-token-value"

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestViewWithoutClientRendersLoadError(t *testing.T) {
	t.Parallel()

	tildePath := "~" + string(filepath.Separator) + filepath.Join(".kube", "config")
	cases := []struct {
		name  string
		setup func(t *testing.T, path string)
		want  []string
	}{
		{
			name:  "missing file",
			setup: func(*testing.T, string) {},
			want:  []string{tildePath, "no such file"},
		},
		{
			name: "bad yaml",
			setup: func(t *testing.T, path string) {
				writeFile(t, path, "apiVersion: v1\nkind: Config\nusers:\n- name: u1\n  user: {token: "+fakeToken+"\n")
			},
			want: []string{tildePath},
		},
		{
			name: "missing current-context",
			setup: func(t *testing.T, path string) {
				writeFile(t, path, "apiVersion: v1\nkind: Config\ncurrent-context: \"\"\nusers:\n- name: u1\n  user: {token: "+fakeToken+"}\n")
			},
			want: []string{"no current-context"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := filepath.Join(home, ".kube", "config")
			tc.setup(t, path)
			_, loadErr := k8s.LoadFromKubeconfig(path)
			if loadErr == nil {
				t.Fatal("LoadFromKubeconfig succeeded, want an error")
			}

			out := newModel(nil, loadErr, home).View()

			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("view missing %q:\n%s", w, out)
				}
			}
			for _, bad := range []string{home, fakeToken, "make k3s-up"} {
				if strings.Contains(out, bad) {
					t.Errorf("view must not contain %q:\n%s", bad, out)
				}
			}
			if !strings.Contains(out, connectHint) {
				t.Errorf("view missing the generic hint %q:\n%s", connectHint, out)
			}
		})
	}
}

func TestViewWithoutClientOrErrorShowsHint(t *testing.T) {
	t.Parallel()

	out := New(nil, nil).View()
	if !strings.Contains(out, "no kubeconfig") || !strings.Contains(out, connectHint) {
		t.Fatalf("view = %q, want the no-kubeconfig message and hint", out)
	}
	if strings.Contains(out, "make k3s-up") {
		t.Fatalf("view still shows the dev-only hint: %q", out)
	}
}

func TestTildeHome(t *testing.T) {
	t.Parallel()

	sep := string(filepath.Separator)
	home := sep + filepath.Join("home", "al")
	cases := []struct {
		name string
		in   string
		home string
		want string
	}{
		{name: "path under home", in: "stat " + home + sep + "x: gone", home: home, want: "stat ~" + sep + "x: gone"},
		{name: "every occurrence", in: home + sep + "a " + home + sep + "b", home: home, want: "~" + sep + "a ~" + sep + "b"},
		{name: "trailing separator on home", in: home + sep + "a", home: home + sep, want: "~" + sep + "a"},
		{name: "sibling dir sharing the prefix", in: home + "ice" + sep + "a", home: home, want: home + "ice" + sep + "a"},
		{name: "unknown home", in: home + sep + "a", home: "", want: home + sep + "a"},
		{name: "root home is not shortened", in: sep + "etc" + sep + "a", home: sep, want: sep + "etc" + sep + "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tildeHome(tc.in, tc.home); got != tc.want {
				t.Fatalf("tildeHome(%q, %q) = %q, want %q", tc.in, tc.home, got, tc.want)
			}
		})
	}
}

func TestOneLine(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", maxReasonLen+50)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "short single line", in: "context \"a\" not found", want: "context \"a\" not found"},
		{name: "newlines and runs of space collapse", in: "yaml: line 3:\n\t  mapping values\nnot allowed", want: "yaml: line 3: mapping values not allowed"},
		{name: "long text is capped", in: long, want: strings.Repeat("x", maxReasonLen) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := oneLine(tc.in); got != tc.want {
				t.Fatalf("oneLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
