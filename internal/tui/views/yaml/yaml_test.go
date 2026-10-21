package yaml_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/yaml"
)

func ref(kind, ns, name string) views.ResourceRef {
	return views.ResourceRef{Kind: kind, Namespace: ns, Name: name}
}

func TestYAMLOfEachKind(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		ref     views.ResourceRef
		kubectl string
		title   string
		want    []string
	}{
		{
			ref: ref("pod", demo, tuitest.PodWeb1), kubectl: "kubectl get pod web-1 -n demo -o yaml",
			title: "pod · web-1 · yaml",
			want:  []string{"apiVersion: v1", "kind: Pod", "  name: web-1", "  namespace: demo"},
		},
		{
			ref: ref("deployment", demo, tuitest.DeploymentWeb), kubectl: "kubectl get deployment web -n demo -o yaml",
			title: "deployment · web · yaml",
			want:  []string{"apiVersion: apps/v1", "kind: Deployment"},
		},
		{
			ref: ref("service", demo, tuitest.ServiceWeb), kubectl: "kubectl get service web -n demo -o yaml",
			title: "service · web · yaml",
			want:  []string{"apiVersion: v1", "kind: Service", "port: 80"},
		},
		{
			ref: ref("namespace", "", demo), kubectl: "kubectl get namespace demo -o yaml",
			title: "namespace · demo · yaml",
			want:  []string{"apiVersion: v1", "kind: Namespace", "  name: demo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.ref.Kind, func(t *testing.T) {
			t.Parallel()
			m := yaml.New(tuitest.NewFakeClient(t), tc.ref)
			if !strings.Contains(tuitest.Screen(m), "loading yaml") {
				t.Errorf("before the fetch: %q", tuitest.Screen(m))
			}
			m, _ = tuitest.Mount(t, m, 120, 80)
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks %q:\n%s", w, screen)
				}
			}
			if strings.Contains(screen, "managedFields") {
				t.Errorf("managedFields are shown:\n%s", screen)
			}
			if got := m.Command().String(); got != tc.kubectl || m.Command().Approx {
				t.Errorf("Command = %q (approx %v), want %q", got, m.Command().Approx, tc.kubectl)
			}
			if got := m.Title(); got != tc.title {
				t.Errorf("Title = %q, want %q", got, tc.title)
			}
		})
	}
}

func TestYAMLFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    yaml.Model
		want string
	}{
		{name: "no kubeconfig", m: yaml.New(nil, ref("pod", "demo", "x")), want: "no kubeconfig"},
		{name: "missing object", m: yaml.New(tuitest.NewFakeClient(t), ref("pod", "demo", "nope")), want: "yaml failed: get pod:"},
		{name: "unsupported kind", m: yaml.New(tuitest.NewFakeClient(t), ref("secret", "demo", "x")), want: "yaml failed: unsupported kind for yaml: secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tc.m, 100, 20)
			if screen := tuitest.Screen(m); !strings.Contains(screen, tc.want) {
				t.Errorf("screen = %q, want %q", screen, tc.want)
			}
			if strings.Contains(m.Title(), "more") || strings.Contains(m.Title(), "yaml ·") {
				t.Errorf("a failed YAML shows a scroll hint: %q", m.Title())
			}
		})
	}
}

// TestYAMLIsMadeInert: an annotation can hold any bytes; none of them
// reaches the terminal as a control sequence.
func TestYAMLIsMadeInert(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "evil", Namespace: "demo",
		Annotations: map[string]string{"note": "\x1b]52;c;cm0gLXJmIH4=\x07\x1b[2J\rboom"},
	}}
	m, _ := tuitest.Mount(t, yaml.New(tuitest.NewFakeClient(t, tuitest.WithObjects(pod)), ref("pod", "demo", "evil")), 120, 40)
	frame := tuitest.Frame(m)
	for _, bad := range []string{"\x1b]", "\x07", "\r"} {
		if strings.Contains(frame, bad) {
			t.Errorf("the frame carries %q:\n%q", bad, frame)
		}
	}
	if !strings.Contains(tuitest.Screen(m), "boom") {
		t.Errorf("the annotation is gone:\n%s", tuitest.Screen(m))
	}
}

func TestYAMLReplyForAnotherObjectIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	forWeb1 := tuitest.Collect(t, yaml.New(c, ref("pod", tuitest.NamespaceDemo, tuitest.PodWeb1)).Init())
	web2, _ := tuitest.Drive(t, yaml.New(c, ref("pod", tuitest.NamespaceDemo, tuitest.PodWeb2)), tuitest.Resize(100, 40))
	web2, _ = tuitest.Drive(t, web2, forWeb1...)
	if screen := tuitest.Screen(web2); !strings.Contains(screen, "loading yaml") {
		t.Fatalf("web-1's YAML landed in web-2's view:\n%s", screen)
	}
}

func TestYAMLRefresh(t *testing.T) {
	t.Parallel()

	m := yaml.New(tuitest.NewFakeClient(t), ref("pod", tuitest.NamespaceDemo, tuitest.PodWeb1))
	first := tuitest.Collect(t, m.Init())
	m, again := tuitest.Step(t, m, tuitest.Press("r"))
	if len(again) != 0 {
		t.Errorf("r during the first fetch fetched again:\n%s", tuitest.Describe(again))
	}
	m, _ = tuitest.Drive(t, m, first...)
	if _, out := tuitest.Step(t, m, tuitest.Press("r")); len(out) != 1 {
		t.Errorf("r after the answer should fetch once:\n%s", tuitest.Describe(out))
	}
	if cmd := yaml.New(nil, ref("pod", "demo", "x")).Init(); cmd != nil {
		t.Error("Init without a client should not fetch")
	}
}

// TestYAMLRefreshKeepsTheScroll: r fetches the object again and the text
// stays where the reader was.
func TestYAMLRefreshKeepsTheScroll(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, yaml.New(tuitest.NewFakeClient(t), ref("pod", tuitest.NamespaceDemo, tuitest.PodMulti)), 100, 10)
	m, _ = tuitest.Drive(t, m, tuitest.Press("pgdown"))
	before := tuitest.Screen(m)
	if strings.HasPrefix(before, "apiVersion:") {
		t.Fatalf("pgdown did not scroll:\n%s", before)
	}
	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	if len(out) == 0 {
		t.Fatal("r fetched nothing")
	}
	if after := tuitest.Screen(m); after != before {
		t.Errorf("r moved the text:\n%s\nwant as before:\n%s", after, before)
	}
}

func TestYAMLSearchAndBack(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, yaml.New(tuitest.NewFakeClient(t), ref("pod", tuitest.NamespaceDemo, tuitest.PodMulti)), 100, 10)
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"))
	if !m.CapturesKeys() {
		t.Fatal("the search prompt does not take the keys")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Type("SIDECAR"), tuitest.Enter())
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ SIDECAR") || !strings.Contains(screen, "1/") {
		t.Fatalf("no search bar with matches:\n%s", screen)
	}
	next, _, used := m.Back()
	if !used || next.(yaml.Model).Title() == "" {
		t.Fatal("Esc did not clear the search first")
	}
	if _, _, used := next.(yaml.Model).Back(); used {
		t.Error("Esc without a search did not go back")
	}
}

func TestYAMLStaticContract(t *testing.T) {
	t.Parallel()

	r := ref("service", "demo", "web")
	m := yaml.New(nil, r)
	if m.CapturesKeys() || m.Close() != nil {
		t.Error("the YAML view is read-only and owns nothing")
	}
	if got := strings.Join(tuitest.HelpKeys(m), " "); got != "↑↓/pgup/pgdn / n/N r" {
		t.Errorf("help keys = %q", got)
	}
	var sel views.Selection = m
	if got, ok := sel.Selected(); !ok || got != r {
		t.Errorf("Selected = %+v, %v", got, ok)
	}
	if got := yaml.New(nil, ref("pod", "demo", "-bad")).Command(); !got.IsZero() {
		t.Errorf("a name kubectl would misread gave %q", got.String())
	}
}
