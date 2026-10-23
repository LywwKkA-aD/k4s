package tui

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestEntriesOfOneObjectKeepTheirObject: describe and YAML entries of two
// objects in the same namespace stay apart; the same object again
// replaces its entry.
func TestEntriesOfOneObjectKeepTheirObject(t *testing.T) {
	t.Parallel()

	a := historyEntry{view: viewDescribe, namespace: "demo", ref: views.ResourceRef{Kind: "pod", Namespace: "demo", Name: "a"}}
	b := historyEntry{view: viewDescribe, namespace: "demo", ref: views.ResourceRef{Kind: "pod", Namespace: "demo", Name: "b"}}
	h := pushEntry(pushEntry(nil, a), b)
	if len(h) != 2 || h[0] != a || h[1] != b {
		t.Fatalf("history = %+v, want a then b", h)
	}
	if h = pushEntry(h, b); len(h) != 2 {
		t.Errorf("the same object twice stacked: %+v", h)
	}
}

// TestNoEntryForAViewThatCannotComeBack: nothing, and a view the root
// cannot build again, leave history as it is.
func TestNoEntryForAViewThatCannotComeBack(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
	if _, ok := m.entryOf(nil); ok {
		t.Error("a nil view has an entry")
	}
	if _, ok := m.entryOf(stubView{title: "pod · web-1 · logs"}); ok {
		t.Error("logs have an entry")
	}
	m.current = nil
	if got := m.pushHistory(); len(got.history) != 0 {
		t.Errorf("history = %+v", got.history)
	}
}
