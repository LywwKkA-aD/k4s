package keys_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
)

func TestDefaultBindings(t *testing.T) {
	t.Parallel()

	km := keys.Default()
	cases := []struct {
		name    string
		binding key.Binding
		keys    []string
		help    string
	}{
		{name: "quit", binding: km.Quit, keys: []string{"q"}, help: "q"},
		{name: "force quit", binding: km.ForceQuit, keys: []string{"ctrl+c"}, help: "ctrl+c"},
		{name: "help", binding: km.Help, keys: []string{"?"}, help: "?"},
		{name: "command", binding: km.Command, keys: []string{":"}, help: ":"},
		{name: "back", binding: km.Back, keys: []string{"esc"}, help: "esc"},
		{name: "copy", binding: km.Copy, keys: []string{"y"}, help: "y"},
		{name: "yaml", binding: km.YAML, keys: []string{"o"}, help: "o"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.binding.Keys(); !slices.Equal(got, tc.keys) {
				t.Errorf("keys = %v, want %v", got, tc.keys)
			}
			if h := tc.binding.Help(); h.Key != tc.help || h.Desc == "" {
				t.Errorf("help = %+v, want key %q and a description", h, tc.help)
			}
		})
	}
}
