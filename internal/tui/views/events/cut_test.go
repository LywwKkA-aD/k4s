package events_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestCutAtTheDeadlineSaysWhereItWasCut: a read that ran out of time
// after its first page lists what it read, and the note counts those
// events, not k8s.MaxEvents; the command is approximate.
func TestCutAtTheDeadlineSaysWhereItWasCut(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	firstPageOnly := tuitest.WithReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		if calls.Add(1) > 1 {
			return true, nil, fmt.Errorf("get events: %w", context.DeadlineExceeded)
		}
		list := &corev1.EventList{}
		for i := range 3 {
			list.Items = append(list.Items, *ev(demo, fmt.Sprintf("e-%d", i), "Normal", "Pulled", "pulled", pod(demo, "web-1"), 1, time.Hour, time.Hour))
		}
		list.Continue = "more"
		return true, list, nil
	})
	m, _ := mounted(t, tuitest.NewFakeClient(t, firstPageOnly), demo)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "list cut at 3 events") || strings.Contains(screen, fmt.Sprint(k8s.MaxEvents)) {
		t.Errorf("note:\n%s", screen)
	}
	if got := len(rows(m)); got != 3 {
		t.Errorf("rows = %d, want the 3 read", got)
	}
	if !m.Command().Approx {
		t.Error("a cut list gives an exact command")
	}
}
