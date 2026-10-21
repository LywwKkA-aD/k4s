package tui_test

import (
	"sync"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// clock is a settable time source for tui.WithClock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock {
	return &clock{now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// startedAt is started with the model reading the time from clk.
func startedAt(t *testing.T, c *k8s.Client, clk *clock) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil, tui.WithClock(clk.Now)), screenW, screenH)
	return m
}
