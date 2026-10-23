package views_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestNamespaceFallbackNamesTheListAndTheNamespace(t *testing.T) {
	t.Parallel()

	got := views.NamespaceFallback("pods", "demo")()
	if want := (views.NamespaceFallbackMsg{View: "pods", Namespace: "demo"}); got != want {
		t.Errorf("NamespaceFallback(pods, demo) = %#v, want %#v", got, want)
	}
}
