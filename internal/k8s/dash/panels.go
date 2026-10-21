package dash

import "time"

// Avail is how one data source of a panel stands. A Forbidden source carries
// the exact probe to run; the view renders "—", never 0.
type Avail struct {
	State State
	CanI  Cmd           // set for StateForbidden
	Stale time.Duration // how long a Stale source has been stale, coarsened
}

// ObjectKey names one cluster object; the view keys its row cursor by it so
// a poll that reorders rows never moves the cursor to another object.
type ObjectKey struct {
	Kind      string // kubectl resource word: pod, deployment, node, ...
	Namespace string
	Name      string
}

func (k ObjectKey) String() string {
	if k.Namespace == "" {
		return k.Kind + "/" + k.Name
	}
	return k.Kind + "/" + k.Namespace + "/" + k.Name
}

// Usage of CPU (millicores) or memory (bytes) against allocatable.
type Meter struct {
	Known    bool
	Used     int64
	Capacity int64
	Percent  int
	Sev      Severity
}

// PodsPanel is panel 3.
type PodsPanel struct {
	Avail     Avail
	Cmd       Cmd
	Scope     string // namespace listed; "" is all namespaces
	Total     int
	Phases    PhaseCounts
	Failing   []ReasonCount // failing STATUS words with counts, most first
	NotOK     int           // pods that are not Running-and-ready nor Completed
	PerNode   []NodeCount
	Problems  []PodRow // ▲, △ and · rows, most severe and most recent first
	Restarted []PodRow // up to 3 ◇ rows restarted in the last 24 h
	Missing   []string // Table columns the server did not send; their metrics are dropped
}

// PhaseCounts feeds the phase bar.
type PhaseCounts struct {
	Running, Completed, Waiting, Failing, Evicted, Other int
}

// ReasonCount is one STATUS word and how many pods show it.
type ReasonCount struct {
	Reason string
	Count  int
}

// NodeCount is the pods scheduled to one node, from the Table NODE column.
type NodeCount struct {
	Node string
	Pods int
}

// PodRow is one pod row. Namespace is empty, and Resolved false, until a
// typed lookup placed it: a Table without objects carries no namespace.
type PodRow struct {
	Key         ObjectKey
	Resolved    bool
	Ready       int
	ReadyOf     int
	ReadyKnown  bool
	Status      string
	Class       Class
	Sev         Severity
	Restarts    int
	LastRestart time.Duration // coarsened; valid when HasRestart
	HasRestart  bool
	Age         time.Duration // coarsened
	Node        string
	Container   string // logs target from the typed lookup: the failing container, else the first
	Init        bool   // Container is an init container
	Starting    bool   // still inside the start grace, so no verdict chip
	Cmd         Cmd
}

// WorkloadsPanel is panel 4.
type WorkloadsPanel struct {
	Cmd    Cmd
	Deploy KindSummary
	STS    KindSummary
	DS     KindSummary
	Jobs   JobsSummary
	Cron   CronSummary
	Rows   []WorkloadRow // not ready, failed and suspended, most severe first
}

// KindSummary is one ready bar: workloads of a kind fully ready out of all.
type KindSummary struct {
	Avail Avail
	Ready int
	Total int
}

// JobsSummary is the jobs line.
type JobsSummary struct {
	Avail        Avail
	Cmd          Cmd
	FailedRecent int // failed in the last hour
	Active       int
}

// CronSummary is the cron line.
type CronSummary struct {
	Avail        Avail
	Cmd          Cmd
	Total        int
	Suspended    int
	LastSchedule time.Duration // most recent schedule across all, coarsened
	HasLast      bool
}

// WorkloadRow is a deploy, sts, ds, job or cronjob that needs a look.
type WorkloadRow struct {
	Key       ObjectKey
	Resolved  bool
	Ready     int
	Desired   int
	UpToDate  int
	Available int
	Sev       Severity
	Failed    bool          // job: failed
	FailedAgo time.Duration // job: since the Failed condition, coarsened
	Suspended bool          // cronjob
	Cmd       Cmd
}

// EventsPanel is panel 5.
type EventsPanel struct {
	Avail     Avail
	Cmd       Cmd
	Total     int  // warnings listed (or reported, when Truncated)
	Truncated bool // a capped page: the view shows ≥Total
	Reasons   int
	Objects   int
	LastHour  [30]int // warnings per 2-minute bucket over the last hour, oldest first
	Counted   int     // warnings that count toward the verdict (see severity rules)
	Rows      []EventRow
}

// EventRow is one Warning event, newest first.
type EventRow struct {
	Object    ObjectKey
	Container string // from involvedObject.fieldPath spec.containers{name}
	Reason    string
	Message   string
	Count     int32
	Ago       time.Duration // coarsened
	Counted   bool          // recent, on an unflagged object that still exists
	Flagged   bool          // another panel already shows its object as a problem
	Cmd       Cmd
}
