package tuitest_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestPasteIsABracketedPaste(t *testing.T) {
	t.Parallel()

	got, ok := tuitest.Paste("pay").(tea.PasteMsg)
	if !ok || got.Content != "pay" {
		t.Errorf("Paste = %#v, want a PasteMsg carrying pay", tuitest.Paste("pay"))
	}
}

type helper []key.Binding

func (h helper) Help() []key.Binding { return h }

func TestHelpKeysListsTheHintKeys(t *testing.T) {
	t.Parallel()

	v := helper{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe")),
		key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
	}
	if got := tuitest.HelpKeys(v); !slices.Equal(got, []string{"enter", "w"}) {
		t.Errorf("HelpKeys = %v", got)
	}
	if got := tuitest.HelpKeys(helper{}); len(got) != 0 {
		t.Errorf("HelpKeys of nothing = %v", got)
	}
}
