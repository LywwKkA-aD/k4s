package logs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestTagsToggleLeavesEarlierModelUntouched pins T-13 for the logs view:
// re-formatting the lines after 't' must not write into a backing array
// that an earlier copy of the model still renders from.
func TestTagsToggleLeavesEarlierModelUntouched(t *testing.T) {
	t.Parallel()

	send := func(m views.View, msgs ...tea.Msg) views.View {
		for _, msg := range msgs {
			m, _ = m.Update(msg)
		}
		return m
	}
	screen := func(m views.View) string { return ansi.Strip(m.View()) }
	size := tea.WindowSizeMsg{Width: 80, Height: 10}

	client := &k8s.Client{Clientset: fake.NewClientset()}
	m := New(client, "ns", []string{"alpha", "beta"}, 10, "")
	t.Cleanup(func() { _ = m.Close() })

	before := send(m, size, logEventsBatch{
		tok:    m.sess.tok,
		events: []logEvent{{Pod: "alpha", Line: "hello"}},
	})
	if got := screen(before); strings.Contains(got, "[alpha]") {
		t.Fatalf("tags should start off, screen:\n%s", got)
	}

	toggled := send(before, tuitest.Press("t"))
	if got := screen(toggled); !strings.Contains(got, "[alpha] hello") {
		t.Fatalf("'t' should tag lines with the pod name, screen:\n%s", got)
	}

	redrawn := send(before, size)
	if got := screen(redrawn); strings.Contains(got, "[alpha]") {
		t.Errorf("the earlier model picked up the toggled tags, screen:\n%s", got)
	}
}
