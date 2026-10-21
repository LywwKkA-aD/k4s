package async_test

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
)

// A poller against an API that stops answering in time: each timeout
// doubles the next interval up to a minute, and the first answer snaps the
// cadence back.
func ExamplePoller() {
	clk := asynctest.NewClock(time.Time{})
	p := async.NewPoller(async.New(), async.Config{Enabled: true, Clock: clk})
	p, _ = p.Begin()

	timeout := context.DeadlineExceeded
	for _, err := range []error{nil, timeout, timeout, timeout, timeout, timeout, nil} {
		var cmd tea.Cmd
		p, cmd = p.Done(p.Token(), err) // the reply arms the next tick
		cmd()                           // the manual clock records the timer
		fmt.Println(p.Hint())
		tick, _ := clk.Fire()
		p, _ = p.Due(tick.(async.TickMsg)) // the tick starts the next poll
	}
	// Output:
	// watch 5s
	// watch 10s (slow API)
	// watch 20s (slow API)
	// watch 40s (slow API)
	// watch 60s (slow API)
	// watch 60s (slow API)
	// watch 5s
}
