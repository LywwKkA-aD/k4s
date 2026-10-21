package dashboard

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Zoom (z) draws the focused panel over the whole body below the banner:
// every row, the wider history, and on panel 5 the whole change ring. A
// panel the size leaves out opens zoomed on its number. Esc leaves the
// zoom before anything else.

// zoomed returns l with panel p over the body below the banner, or l
// as it is when there is no panel to zoom or no room for its box.
func (l layout) zoomed(p dash.Panel) layout {
	if p == dash.PanelOverview || l.kind == kindMinimal || l.h-l.banner < minBox {
		return l
	}
	banner := max(l.banner, min(bannerRows, l.h-minBox))
	l.zoom, l.banner = p, banner
	l.places = []place{{panel: p, x: 0, y: banner, w: l.w, h: l.h - banner}}
	return l
}

// onZoom is z: zoom the focused panel, or leave the zoom.
func (m Model) onZoom() (views.View, tea.Cmd) {
	f := m.mem.focus
	if f.panel == dash.PanelOverview {
		return m, statusCmd(status.Info, "z zooms the focused panel: pick one with tab or 1-7")
	}
	if f.zoom == f.panel {
		f.zoom = dash.PanelOverview
	} else {
		f.zoom = f.panel
	}
	m.mem.focus = f
	m = m.relayout()
	if m.lay.zoom != m.mem.focus.zoom {
		m.mem.focus.zoom = dash.PanelOverview
		return m.relayout(), statusCmd(status.Info, "no room to zoom: widen the terminal")
	}
	return m, nil
}

// zoomHint is the z hint: "zoom" on a panel, "back" while zoomed, ""
// on the banner.
func (m Model) zoomHint() string {
	switch f := m.mem.focus; {
	case f.panel == dash.PanelOverview:
		return ""
	case f.zoom == f.panel:
		return "back"
	}
	return "zoom"
}

// present reports a panel this cluster has, drawn at this size or not.
func (m Model) present(p dash.Panel) bool {
	return slices.Contains(present(m.shown()), p)
}

// reachable reports whether focusing p shows it: on screen, or zoomed
// where the size leaves it out and there is room for its box.
func (m Model) reachable(p dash.Panel) bool {
	if _, ok := m.lay.placeOf(p); ok {
		return true
	}
	return m.present(p) && m.lay.zoomed(p).zoom == p
}
