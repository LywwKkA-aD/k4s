package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPodPaletteIDsAreUniqueAndOrdered(t *testing.T) {
	t.Parallel()
	ids := PodPalette()
	if len(ids) != len(podColors) {
		t.Fatalf("got %d ids, want %d", len(ids), len(podColors))
	}
	seen := make(map[lipgloss.Color]bool, len(ids))
	for i, id := range ids {
		if seen[id] {
			t.Errorf("duplicate pod colour id %s", id)
		}
		seen[id] = true
		if string(id) != podColors[i].Dark {
			t.Errorf("id %d = %s, want %s", i, id, podColors[i].Dark)
		}
	}
}

func TestPodPaletteReturnsFreshSlice(t *testing.T) {
	t.Parallel()
	a := PodPalette()
	a[0] = "#000000"
	if PodPalette()[0] == "#000000" {
		t.Error("PodPalette must not expose shared state")
	}
}

func TestPodColor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		id   lipgloss.Color
		want lipgloss.TerminalColor
	}{
		{name: "first id resolves to its pair", id: lipgloss.Color(podColors[0].Dark), want: podColors[0]},
		{name: "last id resolves to its pair", id: lipgloss.Color(podColors[len(podColors)-1].Dark), want: podColors[len(podColors)-1]},
		{name: "unknown id passes through", id: "#123456", want: lipgloss.Color("#123456")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := PodColor(c.id); got != c.want {
				t.Errorf("PodColor(%s) = %v, want %v", c.id, got, c.want)
			}
		})
	}
}

func TestPodColorIsColourlessInMono(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	use(monoPalette)
	for _, id := range append(PodPalette(), "#123456") {
		if _, ok := PodColor(id).(lipgloss.NoColor); !ok {
			t.Errorf("PodColor(%s) in mono = %v, want NoColor", id, PodColor(id))
		}
	}
}
