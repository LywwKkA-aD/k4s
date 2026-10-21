package k8s

import "strings"

// clusterScoped are the built-in resources that live outside namespaces,
// each with its API group ("" for core). Only a ClusterRole grants them,
// and `kubectl auth can-i` asks about them without -A.
var clusterScoped = map[string]string{
	"nodes":                           "",
	"namespaces":                      "",
	"persistentvolumes":               "",
	"storageclasses":                  "storage.k8s.io",
	"csidrivers":                      "storage.k8s.io",
	"csinodes":                        "storage.k8s.io",
	"volumeattachments":               "storage.k8s.io",
	"clusterroles":                    "rbac.authorization.k8s.io",
	"clusterrolebindings":             "rbac.authorization.k8s.io",
	"customresourcedefinitions":       "apiextensions.k8s.io",
	"priorityclasses":                 "scheduling.k8s.io",
	"ingressclasses":                  "networking.k8s.io",
	"runtimeclasses":                  "node.k8s.io",
	"apiservices":                     "apiregistration.k8s.io",
	"mutatingwebhookconfigurations":   "admissionregistration.k8s.io",
	"validatingwebhookconfigurations": "admissionregistration.k8s.io",
	"certificatesigningrequests":      "certificates.k8s.io",
}

// isClusterScoped reports a built-in cluster-scoped resource named as
// can-i takes it: "nodes", "storageclasses" or
// "storageclasses.storage.k8s.io". A custom resource that shares a
// built-in name in another group is not one.
func isClusterScoped(resource string) bool {
	name, group, _ := strings.Cut(resource, ".")
	g, ok := clusterScoped[name]
	return ok && (group == "" || group == g)
}
