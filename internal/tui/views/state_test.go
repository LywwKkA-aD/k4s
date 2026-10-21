package views_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestRowKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace, name, want string
	}{
		{namespace: "shop", name: "api", want: "shop/api"},
		{namespace: "", name: "kube-system", want: "kube-system"},
	}
	for _, tc := range cases {
		if got := views.RowKey(tc.namespace, tc.name); got != tc.want {
			t.Errorf("RowKey(%q, %q) = %q, want %q", tc.namespace, tc.name, got, tc.want)
		}
	}
}

func TestStateWithoutSelectionKeepsTheRest(t *testing.T) {
	t.Parallel()

	in := views.State{Filter: "pay", Selected: "shop/api", Cursor: 3, Paused: true, Mode: "nodes", Sort: "cpu"}
	got := in.WithoutSelection()
	want := views.State{Filter: "pay", Paused: true, Mode: "nodes", Sort: "cpu"}
	if got != want {
		t.Fatalf("WithoutSelection = %+v, want %+v", got, want)
	}
	if in.Selected != "shop/api" || in.Cursor != 3 {
		t.Fatalf("WithoutSelection changed its receiver: %+v", in)
	}
	if (views.State{}).WithoutSelection() != (views.State{}) {
		t.Fatal("the zero State must stay zero")
	}
}
