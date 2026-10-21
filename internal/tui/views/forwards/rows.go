package forwards

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/table"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

func columns() []table.Column {
	return []table.Column{
		{Title: "STATUS", Width: 10},
		{Title: "KIND", Width: 11},
		{Title: "NAMESPACE", Width: 16},
		{Title: "NAME", Width: 26},
		{Title: "LOCAL → REMOTE", Width: 18},
		{Title: "DETAIL", Width: 42},
	}
}

// show replaces the rows with list. The cursor stays on the forward it was
// on while that forward is listed, else on the same index, clamped.
func (m *Model) show(list []forwards.Active) {
	prev, _ := m.selected()
	rows := make([]table.Row, 0, len(list))
	cursor := m.table.Cursor()
	now := m.now()
	for i, a := range list {
		rows = append(rows, rowFor(a, now))
		if prev.Forward.ID != "" && a.Forward.ID == prev.Forward.ID {
			cursor = i
		}
	}
	m.shown = list
	m.table.SetRows(rows)
	// SetRows leaves the cursor at -1 once the table was empty.
	m.table.SetCursor(cursor)
}

// selected is the forward under the cursor, as shown.
func (m Model) selected() (forwards.Active, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.shown) {
		return forwards.Active{}, false
	}
	return m.shown[i], true
}

// rowFor lays out one forward as of now. Names come from the state file and
// errors from the cluster, so every cell is sanitized.
func rowFor(a forwards.Active, now time.Time) table.Row {
	return table.Row{
		a.Status.String(),
		sanitize.Line(a.Forward.Kind),
		sanitize.Line(a.Forward.Namespace),
		sanitize.Line(a.Forward.Name),
		ports(a.Forward),
		detail(a, now),
	}
}

// detail is the latest connection failure of a forward and how long ago it
// was, else the error a failed forward hit, or asks the user to confirm a
// forward saved before k4s recorded which cluster it was for. A failure
// the kubelet reports ends the session as "lost connection to pod", so
// the connection's own reason is the one worth the cell.
func detail(a forwards.Active, now time.Time) string {
	switch {
	case a.TunnelErr != "":
		return fmt.Sprintf("last error: %s · %s ago", sanitize.Line(a.TunnelErr), duration.HumanDuration(now.Sub(a.TunnelErrAt)))
	case a.Err != nil:
		return sanitize.Line(a.Err.Error())
	case a.Unbound:
		return forwards.UnknownCluster
	}
	return ""
}

func ports(f forwards.Forward) string {
	return fmt.Sprintf("%d → %d", f.LocalPort, f.RemotePort)
}

// target names a forward the way kubectl port-forward does: kind/name.
func target(f forwards.Forward) string {
	return f.Kind + "/" + f.Name
}

// portForwardCmd is `kubectl port-forward -n <ns> <kind>/<name> <l>:<r>`.
// It does what k4s does: a service port maps to its targetPort on one
// ready pod, as in kubectl. The Manager admits only names kubectl accepts.
func portForwardCmd(f forwards.Forward) kubectl.Cmd {
	return kubectl.New("port-forward", "-n", f.Namespace, target(f),
		fmt.Sprintf("%d:%d", f.LocalPort, f.RemotePort))
}
