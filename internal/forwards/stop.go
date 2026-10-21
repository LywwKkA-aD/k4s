package forwards

import (
	"errors"
	"fmt"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// stopDeadline bounds how long a stop waits for closed sessions to wind
// down. A forwarder whose connection hangs on close must not hold a key
// press or a context switch for longer.
const stopDeadline = 2 * time.Second

var (
	// ErrNotFound is returned for an id the Manager does not know.
	ErrNotFound = errors.New("not found")
	// ErrStopTimeout is returned when closed sessions did not wind down
	// within the deadline; their local ports may stay bound a while longer.
	ErrStopTimeout = errors.New("stop timed out")
	// ErrStopping is returned by Register and Start while the Manager waits
	// for the previous context's forwards to stop (see After).
	ErrStopping = errors.New("the previous context's forwards are still stopping")
	// ErrRunning is returned by Register for the id of a forward that is
	// starting or running.
	ErrRunning = errors.New("the forward is running")
)

func notFound(id string) error {
	return fmt.Errorf("forward %q %w", id, ErrNotFound)
}

// Stopping is a StopAll under way. Its sessions are closed already; Done
// closes once they have wound down or the deadline passed.
type Stopping struct {
	count int
	done  chan struct{}
	// err is written once, before done closes.
	err error
}

// Count is how many live sessions the stop closed.
func (s *Stopping) Count() int { return s.count }

// Done is closed when the stop has ended.
func (s *Stopping) Done() <-chan struct{} { return s.done }

// Wait blocks until the stop has ended and reports how: nil, or a wrapped
// ErrStopTimeout naming how many sessions were still closing.
func (s *Stopping) Wait() error {
	<-s.done
	return s.err
}

// Blocked reports why Register and Start are refused right now: ErrStopping
// while the stop the Manager was built After is under way, nil otherwise.
func (m *Manager) Blocked() error {
	if m.after == nil {
		return nil
	}
	select {
	case <-m.after.done:
		return nil
	default:
		return ErrStopping
	}
}

// BeginStopAll marks every forward stopped, closes every live session and
// returns without waiting for them: the returned Stopping ends when they
// have all wound down, or after one deadline shared by all of them, and the
// stop this Manager was built After has ended too: a second context switch
// must not open the gate while the first switch's stop still runs.
func (m *Manager) BeginStopAll() *Stopping {
	m.mu.Lock()
	var sessions []*k8s.PortForwardSession
	for _, a := range m.active {
		if a.session != nil {
			sessions = append(sessions, a.session)
		}
		a.Status = StatusStopped
		a.session = nil
		a.Err = nil
		a.ended = k8s.TunnelError{}
	}
	m.notify()
	m.mu.Unlock()

	for _, sess := range sessions {
		sess.Close()
	}
	s := &Stopping{count: len(sessions), done: make(chan struct{})}
	if len(sessions) == 0 && m.Blocked() == nil {
		close(s.done)
		return s
	}
	go func() {
		if len(sessions) > 0 {
			s.err = awaitClosed(sessions, m.stopWait)
		}
		if m.after != nil {
			<-m.after.done
		}
		close(s.done)
	}()
	return s
}

// StopAll terminates every live forward and waits for their listeners to
// close, at most one deadline for all of them. Use it when switching
// contexts so the old cluster's forwards do not pin local ports after the
// user has moved on.
func (m *Manager) StopAll() error {
	return m.BeginStopAll().Wait()
}

// Stop signals the running session to terminate and waits for it to wind
// down. It does NOT remove the forward from State: the user still wants to
// remember the intent. Stopping a forward that is not running only clears
// its error.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	a, ok := m.active[id]
	if !ok {
		m.mu.Unlock()
		return notFound(id)
	}
	sess := a.session
	a.Status = StatusStopped
	a.session = nil
	a.Err = nil
	a.ended = k8s.TunnelError{}
	m.notify()
	m.mu.Unlock()
	if sess == nil {
		return nil
	}
	sess.Close()
	return awaitClosed([]*k8s.PortForwardSession{sess}, m.stopWait)
}

// Remove stops the forward and deletes it from State. The forward leaves
// the list even when its session hangs, which is reported; when the save
// fails it stays, stopped, so the list keeps matching the file.
func (m *Manager) Remove(id string) error {
	stopErr := m.Stop(id)
	if errors.Is(stopErr, ErrNotFound) {
		stopErr = nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.state.FindByID(id); !ok {
		delete(m.active, id)
		return stopErr
	}
	next := m.state.Remove(id)
	if err := next.saveTo(m.statePath); err != nil {
		return errors.Join(stopErr, fmt.Errorf("save state: %w", err))
	}
	m.state = next
	delete(m.active, id)
	m.notify()
	return stopErr
}

// awaitClosed waits for already closed sessions to wind down, all under one
// deadline.
func awaitClosed(sessions []*k8s.PortForwardSession, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for _, sess := range sessions {
		select {
		case <-sess.Done:
		case <-timer.C:
			return fmt.Errorf("%w: %d of %d forwards still closing", ErrStopTimeout, stillOpen(sessions), len(sessions))
		}
	}
	return nil
}

func stillOpen(sessions []*k8s.PortForwardSession) int {
	n := 0
	for _, sess := range sessions {
		select {
		case <-sess.Done:
		default:
			n++
		}
	}
	return n
}
