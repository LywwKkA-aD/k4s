package views_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestNamespaceFallbackNamesTheNamespace(t *testing.T) {
	t.Parallel()

	got := views.NamespaceFallback("demo")()
	if want := (views.NamespaceFallbackMsg{Namespace: "demo"}); got != want {
		t.Errorf("NamespaceFallback(demo) = %#v, want %#v", got, want)
	}
}
