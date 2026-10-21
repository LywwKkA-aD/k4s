package tui_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// namespacesDenied is RBAC refusing to list namespaces, worded as the
// apiserver words it.
func namespacesDenied() error {
	denial := errors.New(`User "u" cannot list resource "namespaces" in API group "" at the cluster scope`)
	return apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", denial)
}

// TestNamespacesForbiddenOffersThoseUsedThisSession: with listing
// namespaces refused, the namespaces view offers the ones this session
// switched through, the current one first.
func TestNamespacesForbiddenOffersThoseUsedThisSession(t *testing.T) {
	t.Parallel()

	var rbac tuitest.Outage
	m := started(t, tuitest.NewFakeClient(t, rbac.Lists("namespaces", namespacesDenied())))
	m, _ = tuitest.Drive(t, m, then(colon("ns demo"), colon("ns prod"), colon("ns staging"))...)
	rbac.Down()
	m, _ = tuitest.Drive(t, m, colon("ns")...)

	requireView(t, m, "namespaces")
	requireScreen(t, m, "namespace ›")
	if got, want := rowsAfter(m, "<all>"), []string{"staging", "prod", "demo"}; !slices.Equal(got, want) {
		t.Errorf("offered %q, want %q:\n%s", got, want, tuitest.Screen(m))
	}
}

// rowsAfter are the screen lines after the one that reads first, trimmed,
// up to the first blank one.
func rowsAfter(m tui.Model, first string) []string {
	var rows []string
	found := false
	for _, l := range lines(m) {
		l = strings.TrimSpace(l)
		switch {
		case !found:
			found = l == first
		case l == "":
			return rows
		default:
			rows = append(rows, l)
		}
	}
	return rows
}
