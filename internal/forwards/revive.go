package forwards

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrOtherCluster is returned by Start and Revive for a forward saved
	// for another cluster than the Manager's.
	ErrOtherCluster = errors.New("saved for another cluster")
	// ErrUnbound is returned by Start for a forward saved before k4s
	// recorded servers: Revive binds it to this cluster first.
	ErrUnbound = errors.New("saved for an unknown cluster")
)

// refuseForeign explains why f may not start on this cluster as it stands.
// The caller holds m.mu.
func (m *Manager) refuseForeign(f Forward) error {
	switch m.cluster.relation(f) {
	case otherCluster:
		return fmt.Errorf("forward %s/%s is %w (%s), not %s", f.Kind, f.Name, ErrOtherCluster, where(f.Context, f.Server), m.cluster.where())
	case unboundCluster:
		return fmt.Errorf("forward %s/%s is %w; confirm it to bind it to %s", f.Kind, f.Name, ErrUnbound, m.cluster.where())
	}
	return nil
}

// Revive starts a forward the user has confirmed for this cluster: one
// saved before k4s recorded servers is first saved again for this cluster,
// so the next session no longer asks. A forward of another cluster is
// refused with ErrOtherCluster.
func (m *Manager) Revive(ctx context.Context, id string) error {
	if err := m.Blocked(); err != nil {
		return err
	}
	if err := m.adopt(id); err != nil {
		return err
	}
	return m.Start(ctx, id)
}

// adopt binds an unbound forward to this cluster, saving before it changes
// memory. Bound forwards are left as they are.
func (m *Manager) adopt(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.active[id]
	if !ok {
		return notFound(id)
	}
	if m.cluster.relation(a.Forward) != unboundCluster {
		return nil
	}
	bound := m.cluster.bind(a.Forward)
	next := m.state.Upsert(bound)
	if err := next.saveTo(m.statePath); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	m.state = next
	a.Forward = bound
	m.notify()
	return nil
}
