package dashboard_test

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// TestCloseCancelsTheRefreshInFlight: leaving the dashboard during a slow
// first load stops its requests at once instead of letting them run out
// their own timeouts for a view no longer on screen.
func TestCloseCancelsTheRefreshInFlight(t *testing.T) {
	t.Parallel()

	started, cancelled := make(chan struct{}), make(chan struct{})
	fetch := func(ctx context.Context, _ dash.Plan, prev dash.Snapshot) dash.Snapshot {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return prev
	}
	m := dashboard.New(tuitest.NewFakeClient(t), nil, dashboard.WithFetch(fetch))
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init sent no refresh")
	}
	replied := make(chan tea.Msg, 1)
	go func() { replied <- cmd() }()
	<-started
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Close left the refresh running")
	}
	<-replied
}

func TestCloseWithoutAClient(t *testing.T) {
	t.Parallel()

	if err := dashboard.New(nil, nil).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := (dashboard.Model{}).Close(); err != nil {
		t.Fatalf("Close on the zero Model: %v", err)
	}
}
