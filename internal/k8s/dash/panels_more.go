package dash

import "time"

// ClusterPanel is panel 1. Each row carries its own command.
type ClusterPanel struct {
	Cmd        Cmd // kubectl get --raw='/readyz?verbose'
	API        APIRow
	Version    VersionRow
	You        TextRow
	Age        AgeRow
	Namespaces NamespacesRow
	Storage    StorageRow
	Network    NetworkRow
	Server     TextRow
}

// APIRow summarises /readyz?verbose.
type APIRow struct {
	Avail  Avail
	Ready  bool
	Checks int
	Passed int
	Failed []string // names of failing checks
	Etcd   State    // StateOK, StateFailed, or StateUnknown without an etcd check
	Sev    Severity
	Cmd    Cmd
}

// VersionRow is the server version and what it says about the distro.
type VersionRow struct {
	Avail         Avail
	GitVersion    string
	Distro        string // k3s, rke2, eks, gke or ""
	EmbeddedEtcd  bool   // some node carries the etcd role label
	ControlPlanes int
	Cmd           Cmd
}

// TextRow is a row with one value, such as the user or the API host.
type TextRow struct {
	Avail Avail
	Text  string
	Cmd   Cmd
}

// AgeRow is the cluster age, read off the kube-system namespace.
type AgeRow struct {
	Avail Avail
	Age   time.Duration // coarsened
	Known bool
	Cmd   Cmd
}

// NamespacesRow counts namespaces.
type NamespacesRow struct {
	Avail       Avail
	Count       int
	Terminating int
	Cmd         Cmd
}

// StorageRow counts PVCs by phase.
type StorageRow struct {
	Avail   Avail
	Total   int
	Bound   int
	Pending int
	Lost    int
	Sev     Severity
	Cmd     Cmd
}

// NetworkRow counts services and pending LoadBalancers.
type NetworkRow struct {
	Avail          Avail
	Services       int
	ServicesKnown  bool
	LoadBalancers  int
	Pending        int
	PendingObjects []ObjectKey // pending LoadBalancers placed in a namespace
	Cmd            Cmd
}

// NodesPanel is panel 2.
type NodesPanel struct {
	Avail   Avail
	Metrics Avail // metrics.k8s.io nodes; Absent means no metrics-server
	Cmd     Cmd   // kubectl get nodes -o wide
	TopCmd  Cmd   // ≈ kubectl top nodes, for the history rows
	Total   int
	Ready   int
	CPU     Meter
	Memory  Meter
	Rows    []NodeRow
}

// NodeRow is one node.
type NodeRow struct {
	Key           ObjectKey
	Roles         []string
	Condition     string   // Ready, NotReady or Unknown
	Pressures     []string // MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable
	Unschedulable bool
	Sev           Severity
	CPU           Meter
	Memory        Meter
	Pods          int
	PodsCap       int
	PodsKnown     bool
	Age           time.Duration // coarsened
	Kubelet       string
	Etcd          bool // carries node-role.kubernetes.io/etcd
	ControlPlane  bool
	Cmd           Cmd
}

// K3sPanel is panel 6, present only on k3s with kube-system visible.
type K3sPanel struct {
	Shown      bool
	Hidden     string // why it is hidden: "not k3s" or "no kube-system access"
	Cmd        Cmd    // kubectl get deploy,ds -n kube-system
	Components []Component
	ServiceLB  ServiceLBRow
	Helm       HelmRow
	Etcd       EtcdRow
}

// Component is one bundled k3s component matched by name across the
// deployment and daemonset rows.
type Component struct {
	Key     ObjectKey
	Present bool
	Ready   int
	Desired int
	Sev     Severity
	Cmd     Cmd
}

// ServiceLBRow is k3s's ServiceLB: svclb-* daemonsets, one per LoadBalancer.
type ServiceLBRow struct {
	State   State // StateOK with svclb daemonsets, StateAbsent off, StateUnknown with no LoadBalancers
	Ready   int
	Total   int
	Pending int // LoadBalancers still <pending>
	Cmd     Cmd
}

// HelmRow lists the k3s HelmCharts; Failed is derived from their install jobs.
type HelmRow struct {
	Avail  Avail
	Charts []string
	Failed int
	Cmd    Cmd // Approx: the failed count is derived from jobs
}

// EtcdRow is the newest etcd snapshot, or when snapshots were last
// reconciled where the ETCDSnapshotFile API is missing.
type EtcdRow struct {
	Shown      bool
	Reconciled bool          // read from the reconcile annotation, not a snapshot file
	Known      bool          // a time was found; false shows "no snapshot yet"
	Ago        time.Duration // coarsened
	Node       string
	Cmd        Cmd
}

// TopPodsPanel is panel 7.
type TopPodsPanel struct {
	Avail     Avail
	CmdCPU    Cmd
	CmdMemory Cmd
	ByCPU     []PodUsage
	ByMemory  []PodUsage
	Reporting int
}

// PodUsage is one pod's metrics, summed across containers.
type PodUsage struct {
	Key      ObjectKey
	CPUMilli int64
	MemBytes int64
	Cmd      Cmd
}
