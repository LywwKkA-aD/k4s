package dashboard

import (
	"fmt"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

const (
	// clusterLabelW is the cluster panel's label column, and
	// clusterLabelNarrow its width in a panel narrower than clusterWideW:
	// the longest label, "version", and nothing to spare.
	clusterLabelW      = 9
	clusterLabelNarrow = 7
	clusterWideW       = 64
)

// clusterBody draws panel 1.
func clusterBody(f frame, w, h int) panelBody {
	body := panelBody{footer: "ctx " + f.ctx}
	if d := f.snap.Cluster.Version.Distro; d != "" {
		body.info = markGlyphs[markFact] + " " + clean(d)
	}
	labelW := clusterLabelW
	if w < clusterWideW {
		labelW = clusterLabelNarrow
	}
	room := w - labelW - 6 // cursor, glyph, two gaps and a margin
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
		text := f.paint(it.mark) + " " + padRight(it.label, labelW) + "  " + it.valueIn(room)
		body.lines = append(body.lines, f.listLine(text, i == sel))
	}
	if hidden := len(items) - len(body.lines); hidden > 0 {
		body.footer = fmt.Sprintf("+%d · %s", hidden, body.footer)
	}
	return body
}
