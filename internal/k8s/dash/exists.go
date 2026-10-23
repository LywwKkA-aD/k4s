package dash

// Exists reports whether object k is in this snapshot's lists. known is
// false when the list k would be in has not answered, answered last with
// anything but its data, or is not one the dashboard keeps in full (pods,
// nodes, deployments, statefulsets, daemonsets). A row listed across every
// namespace has no namespace (a Table carries none), so it matches k by
// name.
func (s Snapshot) Exists(k ObjectKey) (exists, known bool) {
	e, ok := listOf(k.Kind)
	if !ok {
		return false, false
	}
	if u := s.Ledger[e]; u.State != StateOK || !u.Answered {
		return false, false
	}
	return s.st.has(k), true
}

// listOf is the endpoint that lists every object of kind.
func listOf(kind string) (Endpoint, bool) {
	switch kind {
	case "pod":
		return Pods, true
	case "node":
		return Nodes, true
	case "deployment":
		return Deployments, true
	case "statefulset":
		return StatefulSets, true
	case "daemonset":
		return DaemonSets, true
	}
	return 0, false
}

// has reports a stored row for k.
func (st store) has(k ObjectKey) bool {
	in := func(name, namespace string) bool {
		return name == k.Name && (namespace == "" || k.Namespace == "" || namespace == k.Namespace)
	}
	switch k.Kind {
	case "pod":
		for _, r := range st.pods {
			if in(r.name, r.namespace) {
				return true
			}
		}
	case "node":
		for _, n := range st.nodes {
			if n.name == k.Name {
				return true
			}
		}
	default:
		for _, kr := range workloadKinds(st) {
			if kr.kind != k.Kind {
				continue
			}
			for _, r := range kr.rows {
				if in(r.name, r.namespace) {
					return true
				}
			}
		}
	}
	return false
}
