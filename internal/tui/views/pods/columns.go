package pods

import (
	"strconv"

	"charm.land/bubbles/v2/table"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 6)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	return append(cols,
		table.Column{Title: "NAME", Width: 36},
		table.Column{Title: "READY", Width: 7},
		table.Column{Title: "STATUS", Width: 18},
		table.Column{Title: "RESTARTS", Width: 9},
		table.Column{Title: "AGE", Width: 8},
	)
}

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
			strconv.Itoa(int(p.Restarts)),
			k8s.HumanizeDuration(p.Age),
		))
	}
	return rows
}
