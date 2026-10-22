package nodes_test

// Black-box tests through the root model: :nodes and :no open the view,
// Enter describes a node without -n, o opens its YAML, and Esc brings the
// list back as it was left.

import (
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

const (
	rootW = 200
	rootH = 40
)

func startedRoot(t *testing.T, c *k8s.Client) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil), rootW, rootH)
	return m
}

func rootView(m tui.Model) string {
	return tuitest.HeaderView(m)
}

func requireRootView(t *testing.T, m tui.Model, want string) {
	t.Helper()
	if got := rootView(m); got != want {
		t.Fatalf("view = %q, want %q\n%s", got, want, tuitest.Screen(m))
	}
}

// kubectlRow is the footer's command row.
func kubectlRow(m tui.Model) string {
	lines := strings.Split(tuitest.Footer(m), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func TestColonOpensTheNodesView(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"nodes", "no", "node"} {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, startedRoot(t, client(t)), tuitest.Colon(alias)...)
			requireRootView(t, m, "nodes")
			if got := kubectlRow(m); got != "kubectl get nodes -o wide" {
				t.Errorf("kubectl row = %q", got)
			}
			if !strings.Contains(tuitest.Screen(m), "control-plane,etcd,master") {
				t.Errorf("no nodes listed:\n%s", tuitest.Screen(m))
			}
		})
	}
}

// TestEscFromDescribeBringsNodesBack: the filter and the row survive
// Enter and Esc; a node is described without a namespace. The fake client
// names no kubeconfig file, so the root gives describe no kubectl, and
// only kubectl describes a node: a warning says so while the command row
// keeps the describe to copy. The warning outlives the describe view
// until its TTL runs out; then the footer is the one the list was left
// with, whole.
func TestEscFromDescribeBringsNodesBack(t *testing.T) {
	t.Parallel()

	const warning = "describe node needs kubectl"
	clk := asynctest.NewClock(epoch)
	root, _ := tuitest.Mount(t, tui.New(client(t), nil, tui.WithClock(clk.Now)), rootW, rootH)
	keys := slices.Concat(tuitest.Colon("no"), tuitest.FilterBy("pi"), tuitest.Keys("down"))
	m, _ := tuitest.Drive(t, root, keys...)
	before := tuitest.Footer(m)
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if got := kubectlRow(m); !strings.HasSuffix(got, "kubectl describe node "+nodeOld) {
		t.Errorf("Enter described %q", got)
	}
	if footer := tuitest.Footer(m); !strings.Contains(footer, warning) {
		t.Errorf("no warning that a node needs kubectl:\n%s", footer)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, warning) || strings.Contains(screen, "unsupported kind") {
		t.Errorf("body:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireRootView(t, m, "nodes")
	if footer := tuitest.Footer(m); !strings.Contains(footer, warning) {
		t.Errorf("the warning left with the describe view:\n%s", footer)
	}
	clk.Advance(status.TTL)
	if after := tuitest.Footer(m); after != before {
		t.Errorf("footer once the warning is gone:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ pi") || strings.Contains(screen, nodeServer) {
		t.Errorf("filter lost:\n%s", screen)
	}
}

func TestOOpensTheNodesYAML(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, startedRoot(t, client(t)), slices.Concat(tuitest.Colon("no"), tuitest.Keys("o"))...)
	if got := kubectlRow(m); got != "kubectl get node "+nodeServer+" -o yaml" {
		t.Errorf("kubectl row = %q", got)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "kind: Node") || !strings.Contains(screen, "kubeletVersion: v1.36.5+k3s1") {
		t.Errorf("YAML:\n%s", screen)
	}
}
