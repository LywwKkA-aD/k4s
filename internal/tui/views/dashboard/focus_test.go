package dashboard

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

func rowsOfKeys(keys ...string) []row {
	out := make([]row, 0, len(keys))
	for _, k := range keys {
		out = append(out, row{key: k})
	}
	return out
}

func TestFocusCycle(t *testing.T) {
	t.Parallel()

	order := []dash.Panel{dash.PanelOverview, dash.PanelCluster, dash.PanelNodes, dash.PanelPods}
	tests := []struct {
		name string
		from dash.Panel
		step int
		want dash.Panel
	}{
		{name: "tab moves on", from: dash.PanelCluster, step: +1, want: dash.PanelNodes},
		{name: "tab wraps to the banner", from: dash.PanelPods, step: +1, want: dash.PanelOverview},
		{name: "shift+tab wraps to the last panel", from: dash.PanelOverview, step: -1, want: dash.PanelPods},
		{name: "a hidden panel starts from the banner", from: dash.PanelTopPods, step: +1, want: dash.PanelCluster},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := focus{panel: tt.from, row: "pod/shop/a", index: 2}
			got := f.cycle(order, tt.step)
			if got.panel != tt.want || got.row != "" {
				t.Fatalf("cycle = %+v, want panel %d at panel level", got, tt.want)
			}
		})
	}
}

func TestFocusAtOpensThePanelInTheAccordion(t *testing.T) {
	t.Parallel()

	f := focus{expanded: dash.PanelPods}.at(dash.PanelNodes)
	if f.panel != dash.PanelNodes || f.expanded != dash.PanelNodes {
		t.Fatalf("at(2) = %+v, want nodes focused and expanded", f)
	}
	f = f.at(dash.PanelOverview)
	if f.expanded != dash.PanelNodes {
		t.Fatalf("the banner closed the open panel: %+v", f)
	}
}

func TestFocusRows(t *testing.T) {
	t.Parallel()

	rows := rowsOfKeys("a", "b", "c")
	f := focus{panel: dash.PanelPods}
	f = f.down(rows)
	if f.row != "a" || f.index != 0 {
		t.Fatalf("down from the panel = %+v, want row a", f)
	}
	f = f.down(rows).down(rows).down(rows)
	if f.row != "c" || f.index != 2 {
		t.Fatalf("down past the end = %+v, want it to stay on c", f)
	}
	f = f.up(rows).up(rows)
	if f.row != "a" {
		t.Fatalf("up = %+v, want a", f)
	}
	f = f.up(rows)
	if f.row != "" {
		t.Fatalf("up on the first row = %+v, want the panel level", f)
	}
	if got := (focus{panel: dash.PanelPods}).down(nil); got.row != "" {
		t.Fatalf("down without rows = %+v, want the panel level", got)
	}
}

func TestFocusBackChain(t *testing.T) {
	t.Parallel()

	f := focus{panel: dash.PanelPods, row: "b", index: 1}
	f, used := f.back()
	if !used || f.row != "" || f.panel != dash.PanelPods {
		t.Fatalf("esc on a row = %+v %v, want the panel level", f, used)
	}
	f, used = f.back()
	if !used || f.panel != dash.PanelOverview {
		t.Fatalf("esc on a panel = %+v %v, want the banner", f, used)
	}
	if _, used = f.back(); used {
		t.Fatal("esc on the banner must be left to the root")
	}
}

func TestFocusAnchorKeepsTheObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rows     []row
		wantRow  string
		wantGone string
	}{
		{name: "the row moved", rows: rowsOfKeys("c", "x", "b"), wantRow: "b"},
		{name: "the row is gone", rows: rowsOfKeys("a", "c"), wantRow: "c", wantGone: "b"},
		{name: "the row is gone from the end", rows: rowsOfKeys("a"), wantRow: "a", wantGone: "b"},
		{name: "every row is gone", rows: nil, wantRow: "", wantGone: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := focus{panel: dash.PanelPods, row: "b", index: 1}
			got, gone := f.anchor(tt.rows)
			if got.row != tt.wantRow || gone != tt.wantGone {
				t.Fatalf("anchor = %+v gone %q, want row %q gone %q", got, gone, tt.wantRow, tt.wantGone)
			}
		})
	}
}

func TestGoneText(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"pod/shop/payments-0": "pod shop/payments-0 is gone",
		"node/k3s-agent-1":    "node k3s-agent-1 is gone",
		"chart":               "chart is gone",
	}
	for key, want := range tests {
		if got := goneText(key); got != want {
			t.Errorf("goneText(%q) = %q, want %q", key, got, want)
		}
	}
}
