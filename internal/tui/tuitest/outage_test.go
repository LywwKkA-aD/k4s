package tuitest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestOutageFailsListsOnlyWhileDown(t *testing.T) {
	t.Parallel()

	boom := errors.New("etcd down")
	var o tuitest.Outage
	c := tuitest.NewFakeClient(t, o.Lists("pods", boom))
	ctx := context.Background()

	if _, err := c.ListPods(ctx, ""); err != nil {
		t.Fatalf("list while up: %v", err)
	}
	o.Down()
	if _, err := c.ListPods(ctx, ""); !errors.Is(err, boom) {
		t.Errorf("list while down = %v, want %v", err, boom)
	}
	if _, err := c.ListServices(ctx, ""); err != nil {
		t.Errorf("another resource failed too: %v", err)
	}
	o.Up()
	if _, err := c.ListPods(ctx, ""); err != nil {
		t.Errorf("list after Up: %v", err)
	}
}

func TestOutageTakesMetricsDown(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	c := tuitest.NewFakeClient(t, o.Metrics())
	ctx := context.Background()

	if _, err := c.ListNodeMetrics(ctx); err != nil {
		t.Fatalf("metrics while up: %v", err)
	}
	o.Down()
	if _, err := c.ListNodeMetrics(ctx); err == nil {
		t.Error("metrics while down succeeded")
	}
	o.Up()
	if _, err := c.ListPodMetrics(ctx, ""); err != nil {
		t.Errorf("metrics after Up: %v", err)
	}
}
