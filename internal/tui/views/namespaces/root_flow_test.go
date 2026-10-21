package namespaces_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestMain keeps the root model these tests drive away from the
// developer's home: no saved port-forwards (which would open the restore
// popup) and no real kubeconfig.
func TestMain(m *testing.M) {
	os.Exit(isolated(m))
}

func isolated(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "k4s-namespaces-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state")); err != nil {
		panic(err)
	}
	kubeconfig, err := tuitest.WriteKubeconfig(tmp, tuitest.FakeContext, tuitest.FakeContext)
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		panic(err)
	}
	return m.Run()
}

const (
	payAPI = "pay-api"
	payDB  = "pay-db"
	payWeb = "pay-web"
)

func payClient(t *testing.T) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, tuitest.WithObjects(
		tuitest.Namespace(payAPI), tuitest.Namespace(payDB), tuitest.Namespace(payWeb), tuitest.Namespace("shop"),
	))
}

func colon(name string) []tea.Msg {
	return []tea.Msg{tuitest.Press(":"), tuitest.Type(name), tuitest.Enter()}
}

// rootFooter is the frame under the footer's rule: the hints or status row
// and the kubectl row.
func rootFooter(m tui.Model) string {
	lines := strings.Split(tuitest.Screen(m), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s != "" && strings.Trim(s, "─") == "" {
			return tuitest.TrimLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

func header(m tui.Model) string {
	first, _, _ := strings.Cut(tuitest.Screen(m), "\n")
	return first
}

// leftOnPayWeb opens :ns, filters "pay", puts the cursor on the third
// match, pauses the watch, then picks the row, which opens its pods.
func leftOnPayWeb(t *testing.T, c *k8s.Client) (tui.Model, string) {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(colon("ns"), tuitest.FilterBy("pay"), tuitest.Keys("down", "down", "down", "w"))...)
	before := rootFooter(m)
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); !strings.Contains(h, "ns: "+payWeb) || !strings.Contains(h, "view: pods") {
		t.Fatalf("enter on %s: header %q", payWeb, h)
	}
	return m, before
}

func TestEscBringsTheNamespacesBackAsLeft(t *testing.T) {
	t.Parallel()

	m, before := leftOnPayWeb(t, payClient(t))
	tuitest.Golden(t, "footer_namespaces_left", before+"\n")

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if h := header(m); !strings.Contains(h, "view: namespaces") {
		t.Fatalf("Esc from pods: header %q", h)
	}
	if after := rootFooter(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ pay") || strings.Contains(screen, "shop") {
		t.Errorf("filter not back:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); !strings.Contains(h, "ns: "+payWeb) {
		t.Errorf("the cursor is not back on %s: header %q", payWeb, h)
	}
}

func TestEscAfterTheRowWasDeleted(t *testing.T) {
	t.Parallel()

	c := payClient(t)
	m, _ := leftOnPayWeb(t, c)
	if err := c.Clientset.CoreV1().Namespaces().Delete(context.Background(), payWeb, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete namespace: %v", err)
	}
	m, out := tuitest.Drive(t, m, tuitest.Esc())
	want := views.StatusMsg{Level: status.Info, Text: "namespace " + payWeb + " is gone"}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	if !strings.Contains(rootFooter(m), want.Text) {
		t.Errorf("footer:\n%s", rootFooter(m))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); !strings.Contains(h, "ns: "+payDB) {
		t.Errorf("the cursor should land on the nearest row %s: header %q", payDB, h)
	}
}
