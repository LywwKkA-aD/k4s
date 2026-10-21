package k8s

import "strings"

// Scope is where the objects of a kind live.
type Scope int

// The scopes KindScope tells apart.
const (
	// ScopeUnknown is a kind k4s does not know, such as a custom
	// resource's: it may live in a namespace or not.
	ScopeUnknown Scope = iota
	// ScopeNamespaced is a kind whose objects live in a namespace.
	ScopeNamespaced
	// ScopeCluster is a kind whose objects live outside namespaces.
	ScopeCluster
)

// builtin is a kind k4s knows the scope of: its resource as RBAC and
// kubectl name it, and its API group ("" for core).
type builtin struct {
	kind, resource, group string
	scope                 Scope
}

// builtins are the built-in kinds, and k3s's own, that events name or
// that RBAC refuses most often. Only a ClusterRole grants a cluster-scoped
// one, and `kubectl auth can-i` asks about it without -A.
var builtins = []builtin{
	{"Node", "nodes", "", ScopeCluster},
	{"Namespace", "namespaces", "", ScopeCluster},
	{"PersistentVolume", "persistentvolumes", "", ScopeCluster},
	{"StorageClass", "storageclasses", "storage.k8s.io", ScopeCluster},
	{"CSIDriver", "csidrivers", "storage.k8s.io", ScopeCluster},
	{"CSINode", "csinodes", "storage.k8s.io", ScopeCluster},
	{"VolumeAttachment", "volumeattachments", "storage.k8s.io", ScopeCluster},
	{"ClusterRole", "clusterroles", "rbac.authorization.k8s.io", ScopeCluster},
	{"ClusterRoleBinding", "clusterrolebindings", "rbac.authorization.k8s.io", ScopeCluster},
	{"CustomResourceDefinition", "customresourcedefinitions", "apiextensions.k8s.io", ScopeCluster},
	{"PriorityClass", "priorityclasses", "scheduling.k8s.io", ScopeCluster},
	{"IngressClass", "ingressclasses", "networking.k8s.io", ScopeCluster},
	{"RuntimeClass", "runtimeclasses", "node.k8s.io", ScopeCluster},
	{"APIService", "apiservices", "apiregistration.k8s.io", ScopeCluster},
	{"MutatingWebhookConfiguration", "mutatingwebhookconfigurations", "admissionregistration.k8s.io", ScopeCluster},
	{"ValidatingWebhookConfiguration", "validatingwebhookconfigurations", "admissionregistration.k8s.io", ScopeCluster},
	{"CertificateSigningRequest", "certificatesigningrequests", "certificates.k8s.io", ScopeCluster},
	{"ETCDSnapshotFile", "etcdsnapshotfiles", "k3s.cattle.io", ScopeCluster},

	{"Pod", "pods", "", ScopeNamespaced},
	{"Service", "services", "", ScopeNamespaced},
	{"Endpoints", "endpoints", "", ScopeNamespaced},
	{"ConfigMap", "configmaps", "", ScopeNamespaced},
	{"Secret", "secrets", "", ScopeNamespaced},
	{"PersistentVolumeClaim", "persistentvolumeclaims", "", ScopeNamespaced},
	{"ServiceAccount", "serviceaccounts", "", ScopeNamespaced},
	{"ReplicationController", "replicationcontrollers", "", ScopeNamespaced},
	{"PodTemplate", "podtemplates", "", ScopeNamespaced},
	{"LimitRange", "limitranges", "", ScopeNamespaced},
	{"ResourceQuota", "resourcequotas", "", ScopeNamespaced},
	{"Event", "events", "", ScopeNamespaced},
	{"Event", "events", "events.k8s.io", ScopeNamespaced},
	{"Deployment", "deployments", "apps", ScopeNamespaced},
	{"ReplicaSet", "replicasets", "apps", ScopeNamespaced},
	{"StatefulSet", "statefulsets", "apps", ScopeNamespaced},
	{"DaemonSet", "daemonsets", "apps", ScopeNamespaced},
	{"ControllerRevision", "controllerrevisions", "apps", ScopeNamespaced},
	{"Job", "jobs", "batch", ScopeNamespaced},
	{"CronJob", "cronjobs", "batch", ScopeNamespaced},
	{"HorizontalPodAutoscaler", "horizontalpodautoscalers", "autoscaling", ScopeNamespaced},
	{"PodDisruptionBudget", "poddisruptionbudgets", "policy", ScopeNamespaced},
	{"Ingress", "ingresses", "networking.k8s.io", ScopeNamespaced},
	{"NetworkPolicy", "networkpolicies", "networking.k8s.io", ScopeNamespaced},
	{"EndpointSlice", "endpointslices", "discovery.k8s.io", ScopeNamespaced},
	{"Role", "roles", "rbac.authorization.k8s.io", ScopeNamespaced},
	{"RoleBinding", "rolebindings", "rbac.authorization.k8s.io", ScopeNamespaced},
	{"Lease", "leases", "coordination.k8s.io", ScopeNamespaced},
	{"HelmChart", "helmcharts", "helm.cattle.io", ScopeNamespaced},
	{"HelmChartConfig", "helmchartconfigs", "helm.cattle.io", ScopeNamespaced},
	{"Addon", "addons", "k3s.cattle.io", ScopeNamespaced},
	{"Plan", "plans", "upgrade.cattle.io", ScopeNamespaced},
}

var (
	// kindScopes is builtins by "Kind" or "Kind.group".
	kindScopes = indexBuiltins(func(b builtin) string { return qualified(b.kind, b.group) })
	// resourceScopes is builtins by resource name alone.
	resourceScopes = indexBuiltins(func(b builtin) string { return b.resource })
)

func indexBuiltins(key func(builtin) string) map[string][]builtin {
	out := make(map[string][]builtin, len(builtins))
	for _, b := range builtins {
		out[key(b)] = append(out[key(b)], b)
	}
	return out
}

func qualified(name, group string) string {
	if group == "" {
		return name
	}
	return name + "." + group
}

// KindScope is where the objects of kind in API group ("" for core) live:
// known for the built-in kinds and k3s's own, ScopeUnknown for any other.
func KindScope(group, kind string) Scope {
	if bs := kindScopes[qualified(kind, group)]; len(bs) > 0 {
		return bs[0].scope
	}
	return ScopeUnknown
}

// isClusterScoped reports a built-in cluster-scoped resource named as
// can-i takes it: "nodes", "storageclasses" or
// "storageclasses.storage.k8s.io". A custom resource that shares a
// built-in name in another group is not one.
func isClusterScoped(resource string) bool {
	name, group, _ := strings.Cut(resource, ".")
	for _, b := range resourceScopes[name] {
		if b.scope == ScopeCluster && (group == "" || group == b.group) {
			return true
		}
	}
	return false
}
