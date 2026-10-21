package tui

import (
	"context"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
)

// onRestoreDecision acts on the startup question about saved forwards.
//
// "Revive" is best-effort: each Start call goes through the usual
// port-availability pre-check, so entries whose local port is now
// occupied (or privileged) end up in Error status and the user can
// retry from the forwards view.
func (m Model) onRestoreDecision(msg overlay.RestoreDecisionMsg) Model {
	if msg.Revive {
		m.reviveAll()
	}
	return m
}

// reviveAll kicks off Start for every forward in state that belongs to the
// current context. Each Start blocks briefly on its own goroutine; we don't
// wait. The Manager's change channel will refresh the forwards view automatically.
func (m Model) reviveAll() {
	if m.forwardMgr == nil {
		return
	}
	mgr := m.forwardMgr
	ctxName := currentContextName(m.client)
	for _, fwd := range mgr.State().Forwards {
		if fwd.Context != "" && fwd.Context != ctxName {
			continue
		}
		id := fwd.ID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = mgr.Start(ctx, id)
		}()
	}
}

// forwardsForContext counts persisted forwards that belong to the named
// context. Forwards with an empty context are treated as belonging to every
// context for backward compatibility with state files written before the
// context field was enforced.
func forwardsForContext(forwards []forwards.Forward, ctx string) int {
	n := 0
	for _, f := range forwards {
		if f.Context == "" || f.Context == ctx {
			n++
		}
	}
	return n
}
