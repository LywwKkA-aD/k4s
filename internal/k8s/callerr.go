package k8s

// CallError is a failed API call, named the way RBAC names the permission
// it needed: Classify reads it to word a Forbidden as `kubectl auth can-i
// <verb> <resource> [-n <ns>|-A]` when the server's message does not say.
// Its text is "<verb> <resource>: <err>", and the API error stays
// reachable through it with errors.As and the apierrors helpers.
type CallError struct {
	// Verb is the RBAC verb: "list", "get".
	Verb string
	// Resource is the resource as kubectl auth can-i takes it: "pods",
	// "deployments", or "pods.metrics.k8s.io" where the group is needed.
	Resource string
	// Namespace is the namespace asked; "" for every namespace or for a
	// cluster-scoped resource.
	Namespace string
	// ClusterScoped marks a resource that lives outside namespaces, such
	// as namespaces and nodes.
	ClusterScoped bool
	Err           error
}

func (e *CallError) Error() string {
	return e.Verb + " " + e.Resource + ": " + e.Err.Error()
}

func (e *CallError) Unwrap() error { return e.Err }

// listError wraps err from listing resource in namespace ("" for all).
func listError(resource, namespace string, err error) error {
	return &CallError{Verb: "list", Resource: resource, Namespace: namespace, Err: err}
}
