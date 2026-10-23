package dashboard

import (
	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// VersionMsg is the root's answer to /version for the active client,
// handed to a dashboard built WithServerVersion(nil).
type VersionMsg struct {
	Info *apiversion.Info
}

// givenVersion folds the root's version into s. A refresh that started
// before the answer landed replies with a snapshot without it, so every
// reply goes through here; one that has it already is left as it is.
func givenVersion(s dash.Snapshot, info *apiversion.Info) dash.Snapshot {
	if info == nil || s.Cluster.Version.GitVersion == info.GitVersion {
		return s
	}
	return s.GiveVersion(info.GitVersion, info.Minor)
}

func (m Model) onVersion(msg VersionMsg) Model {
	if m.client == nil || !m.mem.snap.Caps.VersionGiven || msg.Info == nil {
		return m
	}
	m.version = msg.Info
	m.mem.snap = givenVersion(m.mem.snap, m.version)
	return m.relayout()
}
