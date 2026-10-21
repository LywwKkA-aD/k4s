package namespaces_test

import (
	"context"
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

func header(m tui.Model) string {
	first, _, _ := strings.Cut(tuitest.Screen(m), "\n")
	return first
}

// leftOnPayWeb opens :ns, filters "pay", puts the cursor on the third
// match, pauses the watch, then picks the row, which opens its pods.
func leftOnPayWeb(t *testing.T, c *k8s.Client) (tui.Model, string) {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(tuitest.Colon("ns"), tuitest.FilterBy("pay"), tuitest.Keys("down", "down", "down", "w"))...)
	before := tuitest.Footer(m)
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); tuitest.HeaderNamespace(m) != payWeb || tuitest.HeaderView(m) != "pods" {
		t.Fatalf("enter on %s: header %q", payWeb, h)
	}
	return m, before
}

func TestEscBringsTheNamespacesBackAsLeft(t *testing.T) {
	t.Parallel()

	m, before := leftOnPayWeb(t, payClient(t))
	tuitest.Golden(t, "footer_namespaces_left", before+"\n")

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if h := header(m); tuitest.HeaderView(m) != "namespaces" {
		t.Fatalf("Esc from pods: header %q", h)
	}
	if after := tuitest.Footer(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant as left:\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ pay") || strings.Contains(screen, "shop") {
		t.Errorf("filter not back:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); tuitest.HeaderNamespace(m) != payWeb {
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
	if !strings.Contains(tuitest.Footer(m), want.Text) {
		t.Errorf("footer:\n%s", tuitest.Footer(m))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Enter())
	if h := header(m); tuitest.HeaderNamespace(m) != payDB {
		t.Errorf("the cursor should land on the nearest row %s: header %q", payDB, h)
	}
}

// TestEscClearsTheFilterFirst: the first Esc clears a committed filter
// and stays; the next one leaves.
func TestEscClearsTheFilterFirst(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, tui.New(payClient(t), nil), 160, 40)
	m, _ = tuitest.Drive(t, m, slices.Concat(tuitest.Colon("ns"), tuitest.FilterBy("pay"), []tea.Msg{tuitest.Esc()})...)
	if h := header(m); tuitest.HeaderView(m) != "namespaces" {
		t.Fatalf("the first Esc left: header %q", h)
	}
	if screen := tuitest.Screen(m); strings.Contains(screen, "/ pay") || !strings.Contains(screen, "shop") {
		t.Errorf("the first Esc kept the filter:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if h := header(m); tuitest.HeaderView(m) != "dashboard" {
		t.Errorf("the second Esc: header %q", h)
	}
}
