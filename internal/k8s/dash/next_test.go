package dash

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func podKey(ns, name string) ObjectKey { return ObjectKey{Kind: "pod", Namespace: ns, Name: name} }

func problem(ns, name, status string, restarts int, container string, age, last time.Duration) PodRow {
	c := classOf(status, 0, 1, true)
	return PodRow{
		Key: podKey(ns, name), Resolved: ns != "", Status: status, Class: c, Sev: severityOf(c),
		Restarts: restarts, Container: container, Age: age, LastRestart: last, HasRestart: last > 0,
		Cmd: describe("pod", name, ns),
	}
}

func lines(sg []Suggestion) []string {
	out := make([]string, 0, len(sg))
	for _, s := range sg {
		out = append(out, s.String())
	}
	return out
}

// TestNextRules is the spec's "next" table (§4), one row per rule, plus
// the ordering and dedup rules.
func TestNextRules(t *testing.T) {
	t.Parallel()

	stuck := WorkloadRow{Key: ObjectKey{Kind: "deployment", Namespace: "shop", Name: "web"}, Resolved: true, Ready: 0, Desired: 2, Sev: SevFailing,
		Cmd: describe("deployment", "web", "shop")}
	cases := []struct {
		name string
		snap Snapshot
		want []string
	}{
		{
			name: "a crash with restarts reads the previous logs of the crashing container",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("shop", "w-1", "CrashLoopBackOff", 41, "worker", 3*hour, 2*minute)}}},
			want: []string{"kubectl logs w-1 -n shop -c worker --previous    # why does it keep crashing?"},
		},
		{
			name: "an init container crash names the init container",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("ci", "m-1", "Init:CrashLoopBackOff", 5, "migrate-db", hour, 4*minute)}}},
			want: []string{"kubectl logs m-1 -n ci -c migrate-db --previous    # why does it keep crashing?"},
		},
		{
			name: "Error without restarts, or without a known container, is described",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{
				problem("ci", "job-1", "Error", 0, "app", hour, 0),
				problem("ci", "job-2", "CrashLoopBackOff", 3, "", hour, minute),
			}}},
			want: []string{
				"kubectl describe pod job-2 -n ci    # why is this pod failing?",
				"kubectl describe pod job-1 -n ci    # why is this pod failing?",
			},
		},
		{
			name: "an image pull failure is described",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("staging", "api-1", "ImagePullBackOff", 0, "api", 22*minute, 0)}}},
			want: []string{"kubectl describe pod api-1 -n staging    # why can't it pull its image?"},
		},
		{
			name: "pending for two minutes or more is described, younger is left alone",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{
				problem("ci", "old", "Pending", 0, "", 6*minute, 0),
				problem("ci", "new", "ContainerCreating", 0, "", minute, 0),
			}}},
			want: []string{"kubectl describe pod old -n ci    # why is it not starting?"},
		},
		{
			name: "a row not yet placed in a namespace gets no command",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("", "w-1", "CrashLoopBackOff", 4, "worker", hour, minute)}}},
			want: nil,
		},
		{
			name: "a stuck rollout is described unless one of its pods already is",
			snap: Snapshot{
				Pods:      PodsPanel{Problems: []PodRow{problem("shop", "web-6c9f7d8b5-xk2lp", "CrashLoopBackOff", 2, "app", hour, minute)}},
				Workloads: WorkloadsPanel{Rows: []WorkloadRow{stuck, {Key: ObjectKey{Kind: "statefulset", Namespace: "db", Name: "pg"}, Resolved: true, Desired: 1, Sev: SevFailing, Cmd: describe("statefulset", "pg", "db")}}},
			},
			want: []string{
				"kubectl logs web-6c9f7d8b5-xk2lp -n shop -c app --previous    # why does it keep crashing?",
				"kubectl describe statefulset pg -n db    # why is the rollout stuck?",
			},
		},
		{
			name: "a failed job, a sick node and a pending LoadBalancer",
			snap: Snapshot{
				Workloads: WorkloadsPanel{Rows: []WorkloadRow{{Key: ObjectKey{Kind: "job", Namespace: "ci", Name: "e2e"}, Resolved: true, Failed: true, Sev: SevFailing, FailedAgo: 41 * minute, Cmd: describe("job", "e2e", "ci")}}},
				Nodes:     NodesPanel{Rows: []NodeRow{{Key: ObjectKey{Kind: "node", Name: "n1"}, Condition: "Ready", Pressures: []string{"DiskPressure"}, Sev: SevWaiting, Cmd: describe("node", "n1", "")}}},
				Cluster:   ClusterPanel{Network: NetworkRow{PendingObjects: []ObjectKey{{Kind: "service", Namespace: "shop", Name: "lb"}}}},
			},
			want: []string{
				"kubectl describe job e2e -n ci    # why did the job fail?",
				"kubectl describe node n1    # what is wrong with this node?",
				"kubectl describe svc lb -n shop    # why is the LoadBalancer pending?",
			},
		},
		{
			name: "a probe warning reads the container from fieldPath; a recovered one is ignored",
			snap: Snapshot{Events: EventsPanel{Rows: []EventRow{
				{Object: podKey("monitoring", "grafana-1"), Container: "grafana", Reason: "Unhealthy", Flagged: true, Ago: minute},
				{Object: podKey("shop", "api-1"), Container: "api", Reason: "Unhealthy", Ago: 2 * minute},
				{Object: podKey("shop", "web-1"), Container: "web", Reason: "Unhealthy", Counted: true, Ago: 3 * minute},
			}}},
			want: []string{
				"kubectl logs grafana-1 -n monitoring -c grafana    # why does it fail its probe?",
				"kubectl logs web-1 -n shop -c web    # why does it fail its probe?",
			},
		},
		{
			name: "a denied list and an absent metrics API",
			snap: func() Snapshot {
				s := Snapshot{Scope: Scope{Namespace: "shop"}, Nodes: NodesPanel{Metrics: Avail{State: StateAbsent}}}
				s.Ledger[Nodes].State = StateForbidden
				return s
			}(),
			want: []string{
				"kubectl auth can-i --list -n shop    # what am I allowed to see here?",
				"kubectl api-resources --api-group=metrics.k8s.io    # is the metrics API served?",
			},
		},
		{
			name: "at most five, by severity then recency, one per object",
			snap: Snapshot{Pods: PodsPanel{Problems: []PodRow{
				problem("a", "p1", "CrashLoopBackOff", 1, "c", hour, 9*minute),
				problem("a", "p2", "CrashLoopBackOff", 1, "c", hour, 1*minute),
				problem("a", "p3", "Pending", 0, "", 30*minute, 0),
				problem("a", "p4", "CrashLoopBackOff", 1, "c", hour, 5*minute),
				problem("a", "p5", "ImagePullBackOff", 0, "c", 2*minute, 0),
				problem("a", "p6", "CrashLoopBackOff", 1, "c", hour, 7*minute),
				problem("a", "p7", "CrashLoopBackOff", 1, "c", hour, 8*minute),
			}}, Events: EventsPanel{Rows: []EventRow{{Object: podKey("a", "p2"), Container: "c", Reason: "Unhealthy", Counted: true}}}},
			want: []string{
				"kubectl logs p2 -n a -c c --previous    # why does it keep crashing?",
				"kubectl describe pod p5 -n a    # why can't it pull its image?",
				"kubectl logs p4 -n a -c c --previous    # why does it keep crashing?",
				"kubectl logs p6 -n a -c c --previous    # why does it keep crashing?",
				"kubectl logs p7 -n a -c c --previous    # why does it keep crashing?",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := suggest(tc.snap)
			if !slices.Equal(lines(got), tc.want) {
				t.Fatalf("next =\n  %s\nwant\n  %s", strings.Join(lines(got), "\n  "), strings.Join(tc.want, "\n  "))
			}
			for _, sg := range got {
				if !ReadOnly(sg.Cmd) {
					t.Errorf("%s is not read-only", sg.Cmd)
				}
			}
		})
	}
}

func TestEveryCommandIsReadOnly(t *testing.T) {
	t.Parallel()

	for _, cl := range []cluster{busyK3s(fixedNow), allClearK3s(fixedNow), vanillaKind(fixedNow), noMetricsK3s(fixedNow)} {
		s := refresh(t, newAPIServer(t, cl), Scope{})
		cmds := []Cmd{s.Cluster.Cmd, s.Cluster.API.Cmd, s.Cluster.Version.Cmd, s.Cluster.You.Cmd, s.Cluster.Age.Cmd,
			s.Cluster.Namespaces.Cmd, s.Cluster.Storage.Cmd, s.Cluster.Network.Cmd, s.Cluster.Server.Cmd, s.Nodes.Cmd,
			s.Nodes.TopCmd, s.Pods.Cmd, s.Workloads.Cmd, s.Workloads.Jobs.Cmd, s.Workloads.Cron.Cmd, s.Events.Cmd,
			s.K3s.Cmd, s.K3s.ServiceLB.Cmd, s.K3s.Helm.Cmd, s.TopPods.CmdCPU, s.TopPods.CmdMemory, s.Verdict.Cmd, s.Daily.Cmd}
		for _, r := range s.Nodes.Rows {
			cmds = append(cmds, r.Cmd)
		}
		for _, r := range append(s.Pods.Problems, s.Pods.Restarted...) {
			cmds = append(cmds, r.Cmd)
		}
		for _, r := range s.Workloads.Rows {
			cmds = append(cmds, r.Cmd)
		}
		for _, r := range s.Events.Rows {
			cmds = append(cmds, r.Cmd)
		}
		for _, r := range s.K3s.Components {
			cmds = append(cmds, r.Cmd)
		}
		for _, n := range s.Next {
			cmds = append(cmds, n.Cmd)
		}
		for _, c := range cmds {
			if !c.IsZero() && !ReadOnly(c) {
				t.Errorf("%s: %s is not on the read-only allow-list", cl.git, c)
			}
		}
	}
}

func TestCommandOfTheDay(t *testing.T) {
	t.Parallel()

	if len(DailyCommands()) < 30 {
		t.Fatalf("%d curated commands, want about 30", len(DailyCommands()))
	}
	for _, d := range DailyCommands() {
		if !ReadOnly(d.Cmd) || d.Question == "" || !strings.HasSuffix(d.Question, "?") {
			t.Errorf("%s: read-only %v, question %q", d.Cmd, ReadOnly(d.Cmd), d.Question)
		}
	}
	bare := Env{}
	full := Env{Metrics: true, K3s: true, Etcd: true}
	seen := map[string]bool{}
	for day := 1; day <= 366; day++ {
		got := CommandOfTheDay(day, bare)
		for _, d := range DailyCommands() {
			if d.Cmd.String() == got.Cmd.String() && d.Needs != NeedsAny {
				t.Fatalf("day %d suggests %s on a cluster without %v", day, got.Cmd, d.Needs)
			}
		}
		seen[CommandOfTheDay(day, full).Cmd.String()] = true
	}
	if len(seen) != len(DailyCommands()) {
		t.Fatalf("a full year shows %d of %d commands", len(seen), len(DailyCommands()))
	}
	if CommandOfTheDay(0, bare).Action != ActionCopy {
		t.Fatal("the command of the day only copies")
	}
}
