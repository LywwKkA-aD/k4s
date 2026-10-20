// Package k8stest holds test doubles for code built on internal/k8s.
//
// Reactors make a client-go fake clientset answer the way a real apiserver
// does under namespace-scoped RBAC, expired credentials or a stalled
// connection, with the apiserver's own error wording. HTTP handlers plus
// NewServerClient cover what a fake clientset cannot serve at all, such as
// raw REST calls to metrics.k8s.io, and show the exact error client-go
// builds from a given status code.
//
// Import it from _test.go files only.
package k8stest
