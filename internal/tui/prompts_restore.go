package tui

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
)

// forwardsNotice is the status for what mgr had to say about loading the
// saved forwards, such as a corrupt file set aside; "" when there is
// nothing to say. The Manager says it once, so the forwards view, which
// asks too, does not repeat it. The text quotes the file's parse error.
func forwardsNotice(mgr *forwards.Manager) string {
	if mgr == nil {
		return ""
	}
	n := mgr.TakeNotice()
	if n == "" {
		return ""
	}
	return "forwards: " + sanitize.Line(n)
}

// onRestoreDecision acts on the startup question about saved forwards.
//
// "Revive" is best-effort: each Start call goes through the usual
// port-availability pre-check, so entries whose local port is now
// occupied (or privileged) end up in Error status and the user can
// retry from the forwards view.
func (m Model) onRestoreDecision(msg overlay.RestoreDecisionMsg) (Model, tea.Cmd) {
	if !msg.Revive {
		return m, nil
	}
	if m.forwardsStopping() {
		return m.setError(msgForwardsStopping)
	}
	m.reviveAll()
	return m, nil
}

// reviveAll kicks off Revive for every forward the restore question
// offered. Each one blocks briefly on its own goroutine; we don't wait.
// The Manager's change channel will refresh the forwards view
// automatically. A forward saved before k4s recorded servers is saved for
// this cluster as it starts: answering the question confirmed it.
func (m Model) reviveAll() {
	if m.forwardMgr == nil {
		return
	}
	mgr := m.forwardMgr
	for _, a := range restorable(mgr, currentContextName(m.client)) {
		id := a.Forward.ID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = mgr.Revive(ctx, id)
		}()
	}
}

// restoreOverlay asks whether to revive the forwards saved for this
// cluster, naming the clusters they were saved for; nil when there are
// none to offer. The count is what reviveAll starts. A forward saved
// without a context, for a client that knows no server, names nothing.
func restoreOverlay(mgr *forwards.Manager, ctx string, now func() time.Time) overlay.Overlay {
	if mgr == nil {
		return nil
	}
	offered := restorable(mgr, ctx)
	if len(offered) == 0 {
		return nil
	}
	clusters := make([]string, 0, 2)
	for _, a := range offered {
		if w := a.Where(); w != "" && !slices.Contains(clusters, w) {
			clusters = append(clusters, w)
		}
	}
	return overlay.NewRestore(len(offered), now, clusters...)
}

// restorable is what the restore question offers: the forwards the Manager
// lists for its cluster, saved under the context ctx or under none.
func restorable(mgr *forwards.Manager, ctx string) []forwards.Active {
	list := mgr.List()
	out := make([]forwards.Active, 0, len(list))
	for _, a := range list {
		if a.Forward.Context == "" || a.Forward.Context == ctx {
			out = append(out, a)
		}
	}
	return out
}
