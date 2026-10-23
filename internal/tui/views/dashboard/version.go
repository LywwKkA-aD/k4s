package dashboard

import (
	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// VersionMsg is the root's answer to /version for the active client,
// handed to a dashboard built WithServerVersion, with the distribution the
// root detected from it; Err is the request's failure when there is no
// answer. A later answer, from the root's retry, replaces a failure.
type VersionMsg struct {
	Info   *apiversion.Info
	Distro cluster.Distro
	Err    error
}

// givenVersion folds the root's version, or its failure, into s. A
// refresh that started before either landed replies with a snapshot
// without it, so every reply goes through here; one that has it already
// is left as it is.
func givenVersion(s dash.Snapshot, info *apiversion.Info, err error) dash.Snapshot {
	if info == nil {
		return s.GiveVersionError(err)
	}
	if s.Cluster.Version.GitVersion == info.GitVersion {
		return s
	}
	return s.GiveVersion(info.GitVersion, info.Minor)
}

func (m Model) onVersion(msg VersionMsg) Model {
	if m.client == nil {
		return m
	}
	if msg.Distro != "" {
		m.distro = msg.Distro
	}
	if !m.mem.snap.Caps.VersionGiven {
		return m.relayout()
	}
	switch {
	case msg.Info != nil:
		m.version, m.versionErr = msg.Info, nil
	case msg.Err != nil && m.version == nil:
		m.versionErr = msg.Err
	}
	m.mem.snap = givenVersion(m.mem.snap, m.version, m.versionErr)
	return m.relayout()
}

// hintDistro is the distribution hints are worded for: the root's, else,
// while the root has not told one apart, the one the version names.
func (m Model) hintDistro() cluster.Distro {
	if m.distro != "" && m.distro != cluster.Other {
		return m.distro
	}
	if d := cluster.Distro(m.mem.snap.Cluster.Version.Distro); d != "" {
		return d
	}
	return cluster.Other
}
