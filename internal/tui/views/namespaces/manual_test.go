package namespaces_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
)

// denied is RBAC refusing to list namespaces, worded as the apiserver
// words it.
func denied() error {
	denial := errors.New(`User "u" cannot list resource "namespaces" in API group "" at the cluster scope`)
	return apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", denial)
}

// denyingClient refuses to list namespaces while deny is set; its
// kubeconfig context sets demo.
func denyingClient(t *testing.T, deny *atomic.Bool) *k8s.Client {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		if deny.Load() {
			return true, nil, denied()
		}
		return false, nil, nil
	}))
	c.ContextNamespace = tuitest.NamespaceDemo
	return c
}

func manualView(t *testing.T, recent ...string) namespaces.Model {
	t.Helper()
	var deny atomic.Bool
	deny.Store(true)
	m := namespaces.New(denyingClient(t, &deny), namespaces.WithRecent(recent...))
	m, _ = tuitest.Mount(t, m, width, height)
	return m
}

// TestForbiddenListOffersAPrompt: the error, a prompt to type a namespace
// and the namespaces this session used.
func TestForbiddenListOffersAPrompt(t *testing.T) {
	t.Parallel()

	m := manualView(t, "shop", tuitest.NamespaceDemo)
	screen := tuitest.Screen(m)
	for _, want := range []string{
		"forbidden: list namespaces · kubectl auth can-i list namespaces",
		"namespace ›",
		"<all>", tuitest.NamespaceDemo, "shop",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Count(screen, tuitest.NamespaceDemo) != 1 {
		t.Errorf("demo is listed more than once:\n%s", screen)
	}
	if !m.CapturesKeys() {
		t.Error("the prompt does not have the keyboard")
	}
}

func TestPromptSelects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []tea.Msg
		want views.NamespaceSelectedMsg
	}{
		{name: "a typed namespace", keys: []tea.Msg{tuitest.Type("payments"), tuitest.Enter()}, want: views.NamespaceSelectedMsg{Namespace: "payments"}},
		{name: "a pasted namespace", keys: []tea.Msg{tuitest.Paste("payments"), tuitest.Enter()}, want: views.NamespaceSelectedMsg{Namespace: "payments"}},
		{name: "the context namespace under the cursor", keys: []tea.Msg{tuitest.Enter()}, want: views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo}},
		{name: "a row picked while typing nothing", keys: []tea.Msg{tuitest.Press("down"), tuitest.Enter()}, want: views.NamespaceSelectedMsg{Namespace: "shop"}},
		{name: "a row after leaving the prompt", keys: []tea.Msg{tuitest.Esc(), tuitest.Press("up"), tuitest.Enter()}, want: views.NamespaceSelectedMsg{Namespace: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, manualView(t, "shop"), tc.keys...)
			if got := tuitest.Requests(out); len(got) != 1 || got[0] != tc.want {
				t.Errorf("requests %s, want %+v", tuitest.Describe(got), tc.want)
			}
		})
	}
}

func TestPromptRejectsANameThatIsNotOne(t *testing.T) {
	t.Parallel()

	m, out := tuitest.Drive(t, manualView(t), tuitest.Type("Not_A_Namespace"), tuitest.Enter())
	if got := tuitest.Requests(out); len(got) != 1 {
		t.Fatalf("requests %s, want one status", tuitest.Describe(got))
	}
	s, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || s.Level != status.Warn || !strings.Contains(s.Text, `invalid namespace "Not_A_Namespace"`) {
		t.Errorf("status = %+v", s)
	}
	if !m.CapturesKeys() || !strings.Contains(tuitest.Screen(m), "Not_A_Namespace") {
		t.Errorf("the prompt closed or lost the text:\n%s", tuitest.Screen(m))
	}
}

// TestEscLeavesThePromptForTheView: Esc gives the keys back; / takes them
// again.
func TestEscLeavesThePromptForTheView(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, manualView(t), tuitest.Esc())
	if m.CapturesKeys() {
		t.Fatal("Esc left the prompt with the keyboard")
	}
	if got := strings.Join(tuitest.HelpKeys(m), " "); !strings.Contains(got, "/") {
		t.Errorf("help keys = %q, want / to type a namespace", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("abc"))
	if !m.CapturesKeys() || !strings.Contains(tuitest.Screen(m), "abc") {
		t.Errorf("/ did not reopen the prompt:\n%s", tuitest.Screen(m))
	}
}

// TestGrantedLaterShowsTheList: once the list works the prompt goes.
func TestGrantedLaterShowsTheList(t *testing.T) {
	t.Parallel()

	var deny atomic.Bool
	deny.Store(true)
	m, _ := tuitest.Mount(t, namespaces.New(denyingClient(t, &deny)), width, height)
	deny.Store(false)
	m, _ = tuitest.Drive(t, m, tuitest.Esc(), tuitest.Press("r"))
	screen := tuitest.Screen(m)
	if strings.Contains(screen, "namespace ›") || !strings.Contains(screen, tuitest.NamespaceSystem) || m.CapturesKeys() {
		t.Errorf("the granted list:\n%s", screen)
	}
}

// TestForbiddenAfterAListKeepsTheRows: a refusal over rows is a stale list,
// not a prompt.
func TestForbiddenAfterAListKeepsTheRows(t *testing.T) {
	t.Parallel()

	var deny atomic.Bool
	m, _ := tuitest.Mount(t, namespaces.New(denyingClient(t, &deny)), width, height)
	deny.Store(true)
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	screen := tuitest.Screen(m)
	if strings.Contains(screen, "namespace ›") || !strings.Contains(screen, "stale: forbidden: list namespaces") {
		t.Errorf("a refusal over rows:\n%s", screen)
	}
}
