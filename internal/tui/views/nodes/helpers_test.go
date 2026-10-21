package nodes_test

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/nodes"
)

const (
	width  = 160
	height = 30

	rolePrefix = "node-role.kubernetes.io/"
)

var epoch = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// Node names of the fixture, in the order the API lists them.
const (
	nodeServer = "k3s-server"
	nodeAgent  = "pi-agent-1"
	nodeOld    = "pi-agent-2"
)

// node is a ready linux node created 49 hours before the epoch, the time
// the views of these tests measure from.
func node(name, ip, version, arch string, labels map[string]string, taints int) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, CreationTimestamp: metav1.NewTime(epoch.Add(-49 * time.Hour))},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: version, OperatingSystem: "linux", Architecture: arch},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeHostName, Address: name}, {Type: corev1.NodeInternalIP, Address: ip}},
		},
	}
	for i := range taints {
		n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{Key: "k" + string(rune('a'+i)), Effect: corev1.TaintEffectNoSchedule})
	}
	return n
}

// fixture is a small k3s cluster mid-upgrade: a server with embedded
// etcd, an agent and a cordoned agent still on the old kubelet.
func fixture() []runtime.Object {
	old := node(nodeOld, "10.0.0.12", "v1.35.2+k3s1", "arm", nil, 1)
	old.Spec.Unschedulable = true
	return []runtime.Object{
		node(nodeServer, "10.0.0.2", "v1.36.5+k3s1", "amd64", map[string]string{
			rolePrefix + "control-plane": "true", rolePrefix + "etcd": "true", rolePrefix + "master": "true",
		}, 1),
		node(nodeAgent, "10.0.0.11", "v1.36.5+k3s1", "arm64", nil, 0),
		old,
	}
}

func client(t *testing.T, opts ...tuitest.FakeOption) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, append([]tuitest.FakeOption{tuitest.WithObjects(fixture()...)}, opts...)...)
}

// clocked builds a view on a manual clock, so no test waits on a tick and
// AGE counts back from the clock's time.
func clocked(c *k8s.Client) (nodes.Model, *asynctest.Clock) {
	clk := asynctest.NewClock(epoch)
	return nodes.New(c, nodes.WithClock(clk), nodes.WithNow(clk.Now)), clk
}

// mounted is a sized, loaded view on a manual clock, and what loading it
// emitted.
func mounted(t *testing.T, c *k8s.Client) (nodes.Model, []tea.Msg) {
	t.Helper()
	m, _ := clocked(c)
	return tuitest.Mount(t, m, width, height)
}

func screenLines(m nodes.Model) []string {
	var out []string
	for _, l := range strings.Split(tuitest.TrimLines(tuitest.Screen(m)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// rows are the table lines under the header and its rule, trimmed.
func rows(m nodes.Model) []string {
	lines := screenLines(m)
	for i, l := range lines {
		if !strings.Contains(l, "INTERNAL-IP") {
			continue
		}
		out := lines[i+1:]
		if len(out) > 0 && strings.Trim(strings.TrimSpace(out[0]), "─") == "" {
			out = out[1:]
		}
		return out
	}
	return nil
}

func requireStatus(t *testing.T, msgs []tea.Msg, level status.Level, text string) {
	t.Helper()
	want := views.StatusMsg{Level: level, Text: text}
	if !slices.Contains(tuitest.All[views.StatusMsg](msgs), want) {
		t.Fatalf("want status %+v among:\n%s", want, tuitest.Describe(msgs))
	}
}

// helpLabel is the help text of the binding on k.
func helpLabel(t *testing.T, m nodes.Model, k string) string {
	t.Helper()
	for _, b := range m.Help() {
		if slices.Contains(b.Keys(), k) {
			return b.Help().Desc
		}
	}
	t.Fatalf("no %s binding", k)
	return ""
}

// rvLog records the ResourceVersion of every nodes list.
type rvLog struct {
	mu  sync.Mutex
	rvs []string
}

func (l *rvLog) reactor() tuitest.FakeOption {
	return tuitest.WithReactor("list", "nodes", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if la, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
			l.mu.Lock()
			l.rvs = append(l.rvs, la.GetListOptions().ResourceVersion)
			l.mu.Unlock()
		}
		return false, nil, nil
	})
}

func (l *rvLog) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.rvs)
}
