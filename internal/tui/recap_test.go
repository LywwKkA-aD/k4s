package tui

import (
	"fmt"
	"slices"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func getPods(ns string) kubectl.Cmd { return kubectl.New("get", "pods", "-n", ns) }

func TestRecapCollector(t *testing.T) {
	t.Parallel()

	approx := kubectl.New("logs", "-f", "web-1", "-n", "demo")
	approx.Approx = true

	cases := []struct {
		name string
		add  []kubectl.Cmd
		want []string
	}{
		{name: "empty", want: nil},
		{name: "in the order of use", add: []kubectl.Cmd{getPods("a"), getPods("b")}, want: []string{"kubectl get pods -n a", "kubectl get pods -n b"}},
		{name: "a repeat moves to the end", add: []kubectl.Cmd{getPods("a"), getPods("b"), getPods("a")}, want: []string{"kubectl get pods -n b", "kubectl get pods -n a"}},
		{name: "nothing to run is skipped", add: []kubectl.Cmd{{}, getPods("a"), {}}, want: []string{"kubectl get pods -n a"}},
		{name: "approximate commands keep their mark", add: []kubectl.Cmd{approx}, want: []string{"≈ kubectl logs -f web-1 -n demo"}},
		{name: "decorated commands as pasted", add: []kubectl.Cmd{getPods("a").WithContext("prod")}, want: []string{"kubectl --context prod get pods -n a"}},
		{name: "a command that sets a session flag is skipped", add: []kubectl.Cmd{kubectl.New("describe", "pod", "--server=https://evil"), getPods("a")}, want: []string{"kubectl get pods -n a"}},
		{name: "control characters never reach the terminal", add: []kubectl.Cmd{kubectl.New("get", "pods").WithKubeconfig([]string{"/tmp/x\x1b]52;c;ZXZpbA==\x07y"})}, want: []string{"kubectl --kubeconfig '/tmp/xy' get pods"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var r recap
			for _, c := range tc.add {
				r = r.add(c)
			}
			if !slices.Equal(r.lines, tc.want) {
				t.Fatalf("recap = %q, want %q", r.lines, tc.want)
			}
		})
	}
}

// Past maxRecap commands the least recently used go first.
func TestRecapKeepsTheMostRecentFifteen(t *testing.T) {
	t.Parallel()

	var r recap
	for i := range 20 {
		r = r.add(getPods(fmt.Sprintf("ns-%02d", i)))
	}
	steps := []struct {
		add            string
		oldest, newest string
	}{
		{add: "ns-07", oldest: "ns-05", newest: "ns-07"},
		{add: "ns-02", oldest: "ns-06", newest: "ns-02"},
	}
	for _, s := range steps {
		r = r.add(getPods(s.add))
		if len(r.lines) != maxRecap {
			t.Fatalf("after %s: recap holds %d commands, want %d", s.add, len(r.lines), maxRecap)
		}
		if got, want := r.lines[0], "kubectl get pods -n "+s.oldest; got != want {
			t.Errorf("after %s: oldest kept = %q, want %q", s.add, got, want)
		}
		if got, want := r.lines[maxRecap-1], "kubectl get pods -n "+s.newest; got != want {
			t.Errorf("after %s: newest = %q, want %q", s.add, got, want)
		}
	}
}

// add returns a new recap: a copy taken earlier never changes.
func TestRecapIsAValue(t *testing.T) {
	t.Parallel()

	base := recap{}.add(getPods("a")).add(getPods("b"))
	snapshot := slices.Clone(base.lines)
	_ = base.add(getPods("c"))
	_ = base.add(getPods("a"))
	if !slices.Equal(base.lines, snapshot) {
		t.Fatalf("an earlier recap changed: %q, want %q", base.lines, snapshot)
	}
}

func TestNoteRecap(t *testing.T) {
	t.Parallel()

	forward := overlay.ForwardChosenMsg{Request: views.ForwardRequestMsg{Kind: "service", Namespace: "demo", Name: "web"}, Local: 8080, Remote: 80}
	cases := []struct {
		name string
		msg  any
		want []string
	}{
		{name: "exec", msg: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1", Container: "nginx"}, want: []string{"kubectl exec -it -n demo -c nginx web-1 -- sh"}},
		{name: "port-forward", msg: forward, want: []string{"kubectl port-forward -n demo service/web 8080:80"}},
		{name: "a forward kubectl would reject", msg: overlay.ForwardChosenMsg{Request: views.ForwardRequestMsg{Kind: "service", Namespace: "Demo_1", Name: "web"}}, want: nil},
		{name: "an exec kubectl would misread", msg: views.ExecRequestMsg{Namespace: "demo", Pod: "--all"}, want: nil},
		{name: "anything else", msg: views.StatusMsg{Text: "x"}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := New(nil, nil).noteRecap(tc.msg)
			if !slices.Equal(got.Recap(), tc.want) {
				t.Fatalf("recap = %q, want %q", got.Recap(), tc.want)
			}
		})
	}
}
