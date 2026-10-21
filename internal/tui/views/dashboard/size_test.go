package dashboard_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// The wide mockup is 207x58; the body is the terminal minus 6 rows.
const (
	wideW = 207
	wideH = 52
)

// TestEverySizeFits renders sizes from 20x10 to 300x100 (every layout
// threshold and a grid between) with the focus in several places, and
// requires the body to be exactly that size, without a panic.
func TestEverySizeFits(t *testing.T) {
	t.Parallel()

	m := mount(t, newFeed(busyK3s), wideW, wideH)
	focused := make([]dashboard.Model, 0, 5)
	for _, keys := range [][]string{nil, {"3", "down"}, {"2", "down", "down", "down", "down"}, {"1", "down"}, {"7"}} {
		moved, _ := press(t, m, keys...)
		focused = append(focused, moved)
	}
	sizes := [][2]int{{20, 10}, {300, 100}, {39, 5}, {40, 6}, {59, 13}, {60, 14}, {99, 29}, {100, 30}, {159, 43}, {160, 44}}
	for w := 20; w <= 300; w += 23 {
		for h := 10; h <= 100; h += 9 {
			sizes = append(sizes, [2]int{w, h})
		}
	}
	for _, size := range sizes {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			t.Parallel()
			for _, base := range focused {
				sized, _ := base.Update(tuitest.Resize(w, h))
				requireSize(t, tuitest.Frame(sized), w, h)
			}
		})
	}
}

func TestTinySizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		w, h int
		want string
	}{
		{w: 30, h: 4, want: "widen the terminal"},
		{w: 50, h: 12, want: "3 pods"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			t.Parallel()
			screen := tuitest.Screen(mount(t, newFeed(busyK3s), tt.w, tt.h))
			requireSize(t, screen, tt.w, tt.h)
			if !strings.Contains(screen, tt.want) {
				t.Fatalf("screen lacks %q:\n%s", tt.want, screen)
			}
		})
	}
}
