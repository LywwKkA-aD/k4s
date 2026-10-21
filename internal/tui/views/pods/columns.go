package pods

import (
	"charm.land/bubbles/v2/table"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Column widths. READY, STATUS, RESTARTS and AGE hold what kubectl prints
// for them ("CrashLoopBackOff", "3 (5m30s ago)", "7d23h"); NAMESPACE and
// NAME share what the screen has left, within bounds.
const (
	readyWidth    = 5
	statusWidth   = 16
	restartsWidth = 13
	ageWidth      = 6

	minNamespaceWidth = 9
	maxNamespaceWidth = 20
	minNameWidth      = 16
	// maxNameWidth is the longest DNS label, which most pod names fit; a
	// wider NAME only pushes the other columns away from their names.
	maxNameWidth = 63

	// defaultWidth lays the table out before the first WindowSizeMsg.
	defaultWidth = 80
)

// widths are the flexible columns; namespace is 0 without that column.
type widths struct {
	namespace int
	name      int
}

// layout fits the table into width: at 80 columns and up nothing wraps.
// Narrower screens keep the minimums and wrap.
func layout(width int, showNamespace bool) widths {
	if width <= 0 {
		width = defaultWidth
	}
	cols := 5
	if showNamespace {
		cols = 6
	}
	pad := styles.Table().Cell.GetHorizontalFrameSize()
	rest := width - cols*pad - (readyWidth + statusWidth + restartsWidth + ageWidth)
	var w widths
	if showNamespace {
		w.namespace = min(max(rest*2/5, minNamespaceWidth), maxNamespaceWidth)
		rest -= w.namespace
	}
	w.name = min(max(rest, minNameWidth), maxNameWidth)
	return w
}

func (w widths) columns() []table.Column {
	cols := make([]table.Column, 0, 6)
	if w.namespace > 0 {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: w.namespace})
	}
	return append(cols,
		table.Column{Title: "NAME", Width: w.name},
		table.Column{Title: "READY", Width: readyWidth},
		table.Column{Title: "STATUS", Width: statusWidth},
		table.Column{Title: "RESTARTS", Width: restartsWidth},
		table.Column{Title: "AGE", Width: ageWidth},
	)
}

// toRows renders pods as `kubectl get pods` prints them.
func toRows(pods []k8s.Pod, showNamespace bool) []table.Row {
	rows := make([]table.Row, 0, len(pods))
	for _, p := range pods {
		row := make(table.Row, 0, 6)
		if showNamespace {
			row = append(row, p.Namespace)
		}
		rows = append(rows, append(row,
			p.Name,
			p.Ready,
			p.Status,
			p.RestartsText,
			duration.HumanDuration(p.Age),
		))
	}
	return rows
}
