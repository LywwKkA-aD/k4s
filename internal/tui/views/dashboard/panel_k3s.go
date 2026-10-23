package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Panel 6 is k3s's own health, shown only on k3s with kube-system
// visible: the bundled components matched by name across the deploy and
// ds rows, ServiceLB, the helm charts k3s installs and the newest etcd
// snapshot. Every line is a cursor stop with its command. A component
// that is missing is "off": k3s lets each one be disabled, so it is dim,
// never red.

const (
	// k3sInfo is panel 6's top border note.
	k3sInfo = "bundled components"
	// kindColW is the kind column of a component line: "deploy".
	kindColW = 6
)

// k3sItem is one line of panel 6.
type k3sItem struct {
	row
	mark  mark
	label string
	value string
}

// k3sItems are panel 6's lines in order: the components, then ServiceLB,
// helm and etcd where there is something to say.
func k3sItems(s dash.Snapshot) []k3sItem {
	k := s.K3s
	if !k.Shown {
		return nil
	}
	out := make([]k3sItem, 0, len(k.Components)+3)
	for _, c := range k.Components {
		// An off component has no object to describe: its line lists what
		// kube-system runs instead, the panel's command.
		it := k3sItem{
			row:   row{key: "k3s/" + c.Key.String(), cmd: k.Cmd},
			mark:  markOff,
			label: clean(c.Key.Name),
			value: "off",
		}
		if c.Present {
			it.row = row{key: it.key, cmd: c.Cmd, target: c.Key, placed: c.Key.Namespace != ""}
			it.mark = markOf(c.Sev)
			it.value = padRight(shortKind(c.Key.Kind), kindColW) + "  " + fmt.Sprintf("%d/%d", c.Ready, c.Desired)
		}
		out = append(out, it)
	}
	for _, f := range []func(dash.K3sPanel) (k3sItem, bool){serviceLBItem, helmItem, etcdItem} {
		if it, ok := f(k); ok {
			out = append(out, it)
		}
	}
	return out
}

// serviceLBItem is ServiceLB: its svclb DaemonSets when it runs, "off"
// when LoadBalancers wait and none exists, nothing without LoadBalancers.
func serviceLBItem(k dash.K3sPanel) (k3sItem, bool) {
	lb := k.ServiceLB
	it := k3sItem{row: row{key: serviceLBKey, cmd: lb.Cmd}, label: "servicelb"}
	pending := ""
	if lb.Pending > 0 {
		pending = plural(lb.Pending, "LoadBalancer", "LoadBalancers") + " <pending>"
	}
	switch lb.State {
	case dash.StateAbsent:
		it.mark, it.value = markOff, joinNotes("off", pending)
	case dash.StateOK:
		it.mark = markOK
		if lb.Ready < lb.Total {
			it.mark = markWarn
		}
		it.value = joinNotes(fmt.Sprintf("%d/%d svclb ready", lb.Ready, lb.Total), pending)
	default:
		return k3sItem{}, false
	}
	return it, true
}

// helmItem is the charts k3s installs, with the count whose install job
// failed. A denied or absent list hides the line.
func helmItem(k dash.K3sPanel) (k3sItem, bool) {
	h := k.Helm
	if !readable(h.Avail) {
		return k3sItem{}, false
	}
	charts := make([]string, 0, len(h.Charts))
	for _, c := range h.Charts {
		charts = append(charts, clean(c))
	}
	it := k3sItem{row: row{key: "k3s/helm", cmd: h.Cmd}, mark: markOK, label: "helm charts"}
	if h.Failed > 0 {
		it.mark = markErr
	}
	it.value = joinNotes(strings.Join(charts, ", "), fmt.Sprintf("%d failed", h.Failed)) + staleNote(h.Avail)
	return it, true
}

// etcdItem is the newest etcd snapshot, or when the snapshot list was
// last reconciled where the ETCDSnapshotFile API is missing (the
// annotation says no more than that). kine/SQLite servers have no line.
func etcdItem(k dash.K3sPanel) (k3sItem, bool) {
	e := k.Etcd
	if !e.Shown {
		return k3sItem{}, false
	}
	it := k3sItem{row: row{key: "k3s/etcd", cmd: e.Cmd}, mark: markOff, label: "etcd snapshot", value: "none yet"}
	if e.Reconciled {
		it.label = "snapshots reconciled"
	}
	if e.Known {
		it.mark, it.value = markFact, ago(e.Ago)+" ago"
		if e.Node != "" {
			it.value += " on " + clean(e.Node)
		}
	}
	return it, true
}

// k3sBody draws panel 6 as a list.
func k3sBody(f frame, h int) panelBody {
	body := panelBody{info: k3sInfo}
	items := k3sItems(f.snap)
	if len(items) == 0 {
		body.lines = []string{" " + f.paint(markOff) + " " + clean(f.snap.K3s.Hidden)}
		return body
	}
	labelW := 0
	rows := make([]row, 0, len(items))
	for _, it := range items {
		labelW = max(labelW, len([]rune(it.label)))
		rows = append(rows, it.row)
	}
	sel := f.selectedIn(dash.PanelK3s, rows)
	start := window(len(items), h, sel)
	for i := start; i < min(len(items), start+h); i++ {
		it := items[i]
		value := it.value
		if it.mark == markOff {
			value = f.muted(value)
		}
		text := f.paint(it.mark) + " " + padRight(it.label, labelW) + "  " + value
		body.lines = append(body.lines, f.listLine(text, i == sel))
	}
	body.footer = f.listFooter(dash.PanelK3s, len(items), len(items)-len(body.lines))
	return body
}

// k3sSummary is panel 6 on one line, for the accordion: components ready,
// ServiceLB when off, and the etcd snapshot.
func k3sSummary(f frame) string {
	k := f.snap.K3s
	if !k.Shown {
		return f.paint(markOff) + " " + clean(k.Hidden)
	}
	ok, worst := 0, dash.SevOK
	for _, c := range k.Components {
		if c.Present && c.Sev == dash.SevOK {
			ok++
		}
		worst = max(worst, c.Sev)
	}
	parts := []string{f.paint(markOf(worst)) + fmt.Sprintf(" %d/%d components", ok, len(k.Components))}
	if k.ServiceLB.State == dash.StateAbsent {
		parts = append(parts, f.paint(markOff)+" servicelb off")
	}
	if e := k.Etcd; e.Shown && e.Known {
		parts = append(parts, f.paint(markFact)+" etcd "+ago(e.Ago)+" ago")
	}
	return strings.Join(parts, " · ")
}

// k3sStrip is panel 6 as the medium layout's strip: each component by
// name, then ServiceLB, helm and etcd, "● coredns 1/1  ○ servicelb off".
func k3sStrip(f frame) string {
	k := f.snap.K3s
	if !k.Shown {
		return k3sSummary(f)
	}
	parts := make([]string, 0, len(k.Components)+3)
	for _, c := range k.Components {
		name := strings.TrimSuffix(clean(c.Key.Name), "-provisioner")
		if !c.Present {
			parts = append(parts, f.paint(markOff)+" "+name+" off")
			continue
		}
		parts = append(parts, f.paint(markOf(c.Sev))+fmt.Sprintf(" %s %d/%d", name, c.Ready, c.Desired))
	}
	switch lb := k.ServiceLB; lb.State {
	case dash.StateAbsent:
		parts = append(parts, f.paint(markOff)+" servicelb off")
	case dash.StateOK:
		parts = append(parts, f.paint(markOK)+fmt.Sprintf(" servicelb %d/%d", lb.Ready, lb.Total))
	}
	if h := k.Helm; readable(h.Avail) {
		m := markOK
		if h.Failed > 0 {
			m = markErr
		}
		parts = append(parts, f.paint(m)+fmt.Sprintf(" helm %d/%d", len(h.Charts)-h.Failed, len(h.Charts)))
	}
	if e := k.Etcd; e.Shown && e.Known {
		parts = append(parts, f.paint(markFact)+" etcd "+ago(e.Ago)+" ago")
	}
	return strings.Join(parts, "  ")
}
