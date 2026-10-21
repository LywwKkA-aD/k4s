package k8s

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ListOption adjusts the request behind a list function.
type ListOption func(metav1.ListOptions) metav1.ListOptions

// FromCache lets the apiserver answer a list from its watch cache
// (ResourceVersion "0") instead of a consistent read. The answer may lag
// by a moment, which a refresh tick does not mind; on k3s it spares kine
// a full read on every tick. Without it a list is a consistent read, as
// a manual refresh wants.
func FromCache() ListOption {
	return func(o metav1.ListOptions) metav1.ListOptions {
		o.ResourceVersion = "0"
		return o
	}
}

func listOptions(opts []ListOption) metav1.ListOptions {
	var o metav1.ListOptions
	for _, opt := range opts {
		o = opt(o)
	}
	return o
}
