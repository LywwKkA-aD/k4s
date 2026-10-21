package dashboard

import (
	"fmt"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// clusterLabelW is the width of the cluster panel's label column.
const clusterLabelW = 9

// clusterBody draws panel 1.
func clusterBody(f frame, _, h int) panelBody {
	body := panelBody{footer: "ctx " + f.ctx}
	if d := f.snap.Cluster.Version.Distro; d != "" {
		body.info = markGlyphs[markFact] + " " + clean(d)
	}
	items := clusterItems(f.snap)
	sel := -1
	for i, it := range items {
		if f.focus.panel == dash.PanelCluster && f.focus.row == it.key {
			sel = i
		}
	}
	start := window(len(items), h, sel)
	for i := start; i < min(len(items), start+h); i++ {
		it := items[i]
		text := f.paint(it.mark) + " " + padRight(it.label, clusterLabelW) + "  " + it.value
		body.lines = append(body.lines, f.listLine(text, i == sel))
	}
	if hidden := len(items) - len(body.lines); hidden > 0 {
		body.footer = fmt.Sprintf("+%d · %s", hidden, body.footer)
	}
	return body
}
