package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// clusterItem is one line of panel 1: a state glyph, a label, a value and
// the command the line stands for. extra follows the value where there is
// room; short replaces it where there is not.
type clusterItem struct {
	key   string
	mark  mark
	label string
	value string
	extra string
	short string
	cmd   dash.Cmd
}

// valueIn fits the line's value to w columns: with its extra note, then
// without, then the short form, then losing whole " · " parts from the
// end, then cut.
func (it clusterItem) valueIn(w int) string {
	full := it.value
	if it.extra != "" {
		full += " " + it.extra
	}
	for _, v := range []string{full, it.value, it.short} {
		if v != "" && ansi.StringWidth(v) <= w {
			return v
		}
	}
	v := it.value
	if it.short != "" {
		v = it.short
	}
	parts := strings.Split(v, " · ")
	for n := len(parts) - 1; n >= 1; n-- {
		if s := strings.Join(parts[:n], " · "); ansi.StringWidth(s) <= w {
			return s
		}
	}
	return cutText(v, w)
}

// clusterItems are panel 1's lines in order. A line whose source is
// denied says so and carries the auth can-i probe, never a 0; readyz and
// whoami lines are left out on a denial, as the spec asks.
func clusterItems(s dash.Snapshot) []clusterItem {
	c, ns := s.Cluster, clean(s.Scope.Namespace)
	out := make([]clusterItem, 0, 8)
	for _, f := range []func() (clusterItem, bool){
		func() (clusterItem, bool) { return apiItem(c.API) },
		func() (clusterItem, bool) { return versionItem(c.Version) },
		func() (clusterItem, bool) { return textItem("you", markOK, c.You) },
		func() (clusterItem, bool) { return ageItem(c.Age) },
		func() (clusterItem, bool) { return nsItem(c.Namespaces) },
		func() (clusterItem, bool) { return storageItem(c.Storage, ns) },
		func() (clusterItem, bool) { return networkItem(c.Network, ns) },
		func() (clusterItem, bool) { return textItem("server", markNone, c.Server) },
	} {
		if it, ok := f(); ok {
			out = append(out, it)
		}
	}
	return out
}

// readable reports a source with data to show: answered, or stale with
// its last answer kept. A source that went stale before it ever answered
// has nothing to show.
func readable(a dash.Avail) bool {
	return a.State == dash.StateOK || (a.State == dash.StateStale && !a.Unanswered)
}

// staleNote is " · stale 2m" for a source that stopped answering.
func staleNote(a dash.Avail) string {
	if a.State != dash.StateStale {
		return ""
	}
	return " · stale " + ago(a.Stale)
}

func denied(key, what string, a dash.Avail) clusterItem {
	return clusterItem{key: key, mark: markUnknown, label: key, value: what + " forbidden", cmd: a.CanI}
}

func apiItem(r dash.APIRow) (clusterItem, bool) {
	if !readable(r.Avail) {
		return clusterItem{}, false
	}
	it := clusterItem{key: "api", mark: markOK, label: "api", cmd: r.Cmd}
	parts := []string{"ready"}
	if !r.Ready {
		it.mark, parts[0] = markErr, "not ready"
	}
	// The failing checks come first: a narrow panel drops parts from the
	// end, and they are what to look at.
	if len(r.Failed) > 0 {
		parts = append(parts, "failing "+clean(strings.Join(r.Failed, ", ")))
	}
	if r.Checks > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d checks", r.Passed, r.Checks))
	}
	switch {
	case r.Etcd == dash.StateOK:
		parts = append(parts, "etcd ok")
	case r.Etcd == dash.StateFailed && !slices.Contains(r.Failed, "etcd"):
		parts = append(parts, "etcd failing")
	}
	it.value = strings.Join(parts, " · ") + staleNote(r.Avail)
	return it, true
}

func versionItem(v dash.VersionRow) (clusterItem, bool) {
	if v.Avail.State == dash.StateForbidden {
		return denied("version", "version", v.Avail), true
	}
	if !readable(v.Avail) || v.GitVersion == "" {
		return clusterItem{}, false
	}
	parts := []string{clean(v.GitVersion)}
	switch {
	case v.Distro == "k3s" && v.EmbeddedEtcd:
		parts = append(parts, "embedded etcd")
	case v.Distro != "" && v.Distro != "k3s":
		parts = append(parts, clean(v.Distro))
	}
	if v.ControlPlanes > 0 {
		parts = append(parts, plural(v.ControlPlanes, "control plane", "control planes"))
	}
	return clusterItem{key: "version", mark: markFact, label: "version", value: strings.Join(parts, " · "), cmd: v.Cmd}, true
}

// textItem is a one-value line, left out when the value is not known: the
// user name (never SelfSubjectReview's extra) or the API host.
func textItem(key string, m mark, r dash.TextRow) (clusterItem, bool) {
	if !readable(r.Avail) || r.Text == "" {
		return clusterItem{}, false
	}
	return clusterItem{key: key, mark: m, label: key, value: clean(r.Text), cmd: r.Cmd}, true
}

func ageItem(r dash.AgeRow) (clusterItem, bool) {
	if !readable(r.Avail) || !r.Known {
		return clusterItem{}, false
	}
	return clusterItem{key: "age", label: "age", value: ago(r.Age), extra: "(namespace kube-system)", cmd: r.Cmd}, true
}

func nsItem(r dash.NamespacesRow) (clusterItem, bool) {
	if r.Avail.State == dash.StateForbidden {
		it := denied("ns", "namespaces", r.Avail)
		it.value = "forbidden · :ns <name> to switch"
		return it, true
	}
	if !readable(r.Avail) {
		return clusterItem{}, false
	}
	value := fmt.Sprintf("%d · %d terminating", r.Count, r.Terminating) + staleNote(r.Avail)
	return clusterItem{key: "ns", label: "ns", value: value, cmd: r.Cmd}, true
}

// storageItem counts the claims of the namespace listed (ns, "" for all),
// which a namespace-scoped user must see named.
func storageItem(r dash.StorageRow, ns string) (clusterItem, bool) {
	if r.Avail.State == dash.StateForbidden {
		return denied("storage", "pvc", r.Avail), true
	}
	if !readable(r.Avail) {
		return clusterItem{}, false
	}
	it := clusterItem{key: "storage", mark: markOK, label: "storage", cmd: r.Cmd}
	parts := []string{fmt.Sprintf("%d/%d pvc Bound", r.Bound, r.Total)}
	switch {
	case r.Total == 0:
		it.mark, parts[0] = markOff, "no pvc"
	case r.Sev == dash.SevWaiting:
		it.mark = markWarn
	}
	if ns != "" {
		parts[0] += " (ns " + ns + ")"
	}
	if r.Pending > 0 {
		parts = append(parts, fmt.Sprintf("%d Pending", r.Pending))
	}
	if r.Lost > 0 {
		parts = append(parts, fmt.Sprintf("%d Lost", r.Lost))
	}
	it.value = strings.Join(parts, " · ") + staleNote(r.Avail)
	return it, true
}

// networkItem counts the services of the namespace listed (ns, "" for
// all) and its LoadBalancers.
func networkItem(r dash.NetworkRow, ns string) (clusterItem, bool) {
	if r.Avail.State == dash.StateForbidden {
		return denied("network", "services", r.Avail), true
	}
	if !readable(r.Avail) {
		return clusterItem{}, false
	}
	it := clusterItem{key: "network", mark: markOK, label: "network", cmd: r.Cmd}
	var parts []string
	switch {
	case r.ServicesKnown && ns != "":
		parts = append(parts, fmt.Sprintf("%d svc in ns %s", r.Services, ns))
	case r.ServicesKnown:
		parts = append(parts, fmt.Sprintf("%d svc", r.Services))
	}
	switch {
	case r.Pending > 0:
		// The mockup's ○: the balancer is not there yet.
		it.mark = markOff
		parts = append(parts, fmt.Sprintf("%d LoadBalancer <pending>", r.Pending))
	case r.LoadBalancers > 0:
		parts = append(parts, plural(r.LoadBalancers, "LoadBalancer", "LoadBalancers"))
	default:
		parts = append(parts, "no LoadBalancer")
	}
	it.value = strings.Join(parts, " · ") + staleNote(r.Avail)
	it.short = strings.ReplaceAll(it.value, "LoadBalancer", "LB")
	return it, true
}

// clusterSummary is panel 1 on one line: the API, namespaces, storage and
// any pending LoadBalancer.
func clusterSummary(f frame) string {
	var parts []string
	for _, it := range clusterItems(f.snap) {
		short, _, _ := strings.Cut(it.value, " · ")
		switch {
		case it.key == "api":
			parts = append(parts, f.paint(it.mark)+" api "+short)
		case it.mark == markUnknown:
			parts = append(parts, f.paint(it.mark)+" "+it.label+" "+short)
		case it.key == "ns":
			parts = append(parts, fmt.Sprintf("%d ns", f.snap.Cluster.Namespaces.Count))
		case it.key == "storage" && it.mark != markOK:
			parts = append(parts, f.paint(it.mark)+" "+it.value)
		case it.key == "storage":
			parts = append(parts, short)
		case it.key == "network" && it.mark == markOff:
			parts = append(parts, f.paint(markOff)+fmt.Sprintf(" %d LB <pending>", f.snap.Cluster.Network.Pending))
		}
	}
	if len(parts) == 0 {
		return f.unreadable("readyz", f.snap.Cluster.API.Avail)
	}
	return strings.Join(parts, " · ")
}

// plural renders "1 job" or "3 jobs".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
