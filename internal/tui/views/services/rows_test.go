package services_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestCursorComesBackAfterAnEmptyFilter: a query that matched nothing
// leaves no row; editing it back must leave a row under the cursor.
func TestCursorComesBackAfterAnEmptyFilter(t *testing.T) {
	t.Parallel()

	m := mounted(t, payClient(t), shop)
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("zzz"))
	m, _ = tuitest.Drive(t, m, tuitest.Keys("backspace", "backspace", "backspace", "enter")...)
	if got := describes(t, m); got != shop+"/api" {
		t.Fatalf("enter describes %q, want the first row", got)
	}
}

// TestRefreshKeepsTheCursorOnItsRow: a service created above the cursor
// does not move it to another row.
func TestRefreshKeepsTheCursorOnItsRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		change func(t *testing.T, c *k8s.Client)
	}{
		{name: "a service created above", change: func(t *testing.T, c *k8s.Client) {
			createService(t, c, tuitest.Service(shop, "aaa", 80, nil))
		}},
		{name: "a service deleted above", change: func(t *testing.T, c *k8s.Client) {
			deleteService(t, c, shop, "api")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := payClient(t)
			m := mounted(t, c, shop)
			m, _ = tuitest.Drive(t, m, tuitest.Keys("down", "down")...) // pay-db
			tc.change(t, c)
			m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
			if got := describes(t, m); got != shop+"/pay-db" {
				t.Fatalf("enter describes %q after a refresh, want pay-db", got)
			}
		})
	}
}

// TestAgeReadsLikeKubectl: the demo services are 90 minutes old, which
// kubectl prints as 90m, not 1h.
func TestAgeReadsLikeKubectl(t *testing.T) {
	t.Parallel()

	screen := tuitest.Screen(mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo))
	if !strings.Contains(screen, " 90m") {
		t.Errorf("no kubectl-style age in:\n%s", screen)
	}
}

func TestPasteGoesToTheOpenFilter(t *testing.T) {
	t.Parallel()

	m := mounted(t, payClient(t), shop)
	m, _ = tuitest.Drive(t, m, tuitest.Paste("pay"))
	if strings.Contains(tuitest.Screen(m), "/ pay") {
		t.Fatal("a paste with the filter closed reached it")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Paste("payments"), tuitest.Enter())
	if got := describes(t, m); got != shop+"/payments" {
		t.Fatalf("enter describes %q, want the one pasted match", got)
	}
}
