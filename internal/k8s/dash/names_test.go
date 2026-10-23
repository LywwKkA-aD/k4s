package dash

import (
	"testing"
	"time"
)

// A hostile API server names objects as it likes. A name that reads as a
// flag (--server=…) must never reach a command the footer shows and y
// copies: the builder gives no command instead.
const (
	evilName = "--server=https://evil"
	evilNS   = "--kubeconfig=/tmp/evil"
)

func TestBuildersRefuseNamesKubectlWouldMisread(t *testing.T) {
	t.Parallel()

	placed := func(ns, name string) PodRow {
		return PodRow{Key: ObjectKey{Kind: "pod", Namespace: ns, Name: name}, Resolved: true}
	}
	unplaced := func(name string) PodRow { return PodRow{Key: ObjectKey{Kind: "pod", Name: name}} }
	deploy := func(name string) ObjectKey { return ObjectKey{Kind: "deployment", Name: name} }
	cases := []struct {
		name string
		cmd  Cmd
		want string // "" for the zero Cmd
	}{
		{"describe", describe("pod", "web", "shop"), "kubectl describe pod web -n shop"},
		{"describe a node", describe("node", "k3s-server", ""), "kubectl describe node k3s-server"},
		{"describe a name that reads as a flag", describe("pod", evilName, "shop"), ""},
		{"describe in a namespace that reads as a flag", describe("pod", "web", evilNS), ""},
		{"describe a kind from an event that reads as a flag", describe(evilName, "web", "shop"), ""},
		{"describe a kind with upper case", describe("Pod", "web", "shop"), ""},
		{"placed pod", podCmd(placed("shop", "web"), Scope{}), "kubectl describe pod web -n shop"},
		{"placed pod with a hostile name", podCmd(placed("shop", evilName), Scope{}), ""},
		{"unplaced pod", podCmd(unplaced("web"), Scope{}), "kubectl get pods -A --field-selector metadata.name=web"},
		{"unplaced pod with a hostile name", podCmd(unplaced("a,metadata.namespace=b"), Scope{}), ""},
		{"unplaced workload", workloadCmd(deploy("web"), false), "kubectl get deployment -A --field-selector metadata.name=web"},
		{"unplaced workload with a hostile name", workloadCmd(deploy(evilName), false), ""},
		{"scope", withScope("shop", "get", "pods"), "kubectl get pods -n shop"},
		{"scope that reads as a flag", withScope(evilNS, "get", "pods"), ""},
		{"logs", logsCmd(podKey("shop", "web"), "app", "--previous"), "kubectl logs web -n shop -c app --previous"},
		{"logs of a hostile pod", logsCmd(podKey("shop", evilName), "app"), ""},
		{"logs in a hostile namespace", logsCmd(podKey(evilNS, "web"), "app"), ""},
		{"logs of a hostile container", logsCmd(podKey("shop", "web"), evilName), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cmd.String(); tc.want != "" && got != tc.want {
				t.Fatalf("command = %q, want %q", got, tc.want)
			}
			if tc.want == "" && !tc.cmd.IsZero() {
				t.Fatalf("command = %q, want none", tc.cmd.String())
			}
		})
	}
}

// A suggestion is a command and its question: one whose command was
// refused is not shown at all.
func TestSuggestionsWithoutACommandAreDropped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		snap Snapshot
	}{
		{"crash of a hostile pod", Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("shop", evilName, "CrashLoopBackOff", 4, "app", hour, minute)}}}},
		{"crash of a hostile container", Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("shop", "web", "CrashLoopBackOff", 4, evilName, hour, minute)}}}},
		{"pull failure of a hostile pod", Snapshot{Pods: PodsPanel{Problems: []PodRow{problem("shop", evilName, "ImagePullBackOff", 0, "app", hour, 0)}}}},
		{"probe of a hostile container", Snapshot{Events: EventsPanel{Rows: []EventRow{{
			Object: podKey("shop", "web"), Container: evilName, Reason: "Unhealthy", Counted: true,
		}}}}},
		{"pending service in a hostile namespace", Snapshot{Cluster: ClusterPanel{Network: NetworkRow{
			PendingObjects: []ObjectKey{{Kind: "svc", Namespace: evilNS, Name: "lb"}},
		}}}},
		{"denied list in a hostile scope", Snapshot{Scope: Scope{Namespace: evilNS}, Ledger: deniedPods()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := suggest(tc.snap); len(got) != 0 {
				t.Fatalf("suggest = %q, want none", lines(got))
			}
		})
	}
}

func deniedPods() Ledger {
	var led Ledger
	led[Pods] = Usage{Fetched: true, State: StateForbidden}
	return led
}

func TestEventRowsOfAHostileKindHaveNoCommand(t *testing.T) {
	t.Parallel()

	st := store{events: []eventInfo{
		{kind: evilName, namespace: "shop", name: "web", reason: "Failed", count: 1, last: fixedNow.Add(-time.Minute)},
		{kind: "Pod", namespace: "shop", name: "web", reason: "Failed", count: 1, last: fixedNow.Add(-2 * time.Minute)},
	}}
	p := deriveEvents(st, Ledger{}, Scope{}, flagged{}, fixedNow)
	if len(p.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(p.Rows))
	}
	if !p.Rows[0].Cmd.IsZero() {
		t.Errorf("hostile kind: command = %q, want none", p.Rows[0].Cmd.String())
	}
	if got, want := p.Rows[1].Cmd.String(), "kubectl describe pod web -n shop"; got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestEtcdRowOfAHostileNodeHasNoCommand(t *testing.T) {
	t.Parallel()

	var led Ledger
	led[EtcdSnapshots] = Usage{Fetched: true, State: StateAbsent}
	for _, tc := range []struct {
		name, node string
		zero       bool
	}{
		{"node", "k3s-server", false},
		{"hostile node", evilName, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := etcdRow(store{nodes: []nodeInfo{{name: tc.node, etcd: true}}}, led, fixedNow)
			if !row.Shown || row.Cmd.IsZero() != tc.zero {
				t.Fatalf("row = %+v, want a command: %v", row, !tc.zero)
			}
		})
	}
}
