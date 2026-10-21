package dash

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

func stuckDeployment(ns, name string) WorkloadRow {
	return WorkloadRow{
		Key: ObjectKey{Kind: "deployment", Namespace: ns, Name: name}, Resolved: true, Ready: 0, Desired: 2, Sev: SevFailing,
		Cmd: describe("deployment", name, ns),
	}
}

func TestOwnedBy(t *testing.T) {
	t.Parallel()

	api := ObjectKey{Kind: "deployment", Namespace: "shop", Name: "api"}
	pg := ObjectKey{Kind: "statefulset", Namespace: "shop", Name: "pg"}
	cases := []struct {
		name string
		pod  ObjectKey
		w    ObjectKey
		want bool
	}{
		{"deployment pod", podKey("shop", "api-7d9f-x2k"), api, true},
		{"statefulset pod", podKey("shop", "pg-0"), pg, true},
		{"another workload with the same prefix", podKey("shop", "api-gateway-7d9f-x2k"), api, false},
		{"a statefulset prefix that is not an ordinal", podKey("shop", "pg-replica-0"), pg, false},
		{"another namespace", podKey("ops", "api-7d9f-x2k"), api, false},
		{"not a pod", ObjectKey{Kind: "job", Namespace: "shop", Name: "api-7d9f-x2k"}, api, false},
		{"the bare workload name", podKey("shop", "api"), api, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ownedBy(tc.pod, tc.w); got != tc.want {
				t.Fatalf("ownedBy(%v, %v) = %v, want %v", tc.pod.Name, tc.w.Name, got, tc.want)
			}
		})
	}
}

func TestNextKeepsAWorkloadWhenOnlyAnUnrelatedPodMatchesItsPrefix(t *testing.T) {
	t.Parallel()

	s := Snapshot{
		Pods:      PodsPanel{Problems: []PodRow{problem("shop", "api-gateway-7d9f-x2k", "CrashLoopBackOff", 2, "app", hour, minute)}},
		Workloads: WorkloadsPanel{Rows: []WorkloadRow{stuckDeployment("shop", "api")}},
	}
	got := lines(suggest(s))
	want := "kubectl describe deployment api -n shop    # why is the rollout stuck?"
	if !slices.Contains(got, want) {
		t.Fatalf("suggestions %q lack %q", got, want)
	}
}

func TestNextCoverageCountsOnlyPickedPods(t *testing.T) {
	t.Parallel()

	var pods []PodRow
	for i := range maxSuggestions {
		p := problem("ops", fmt.Sprintf("crash-%d", i), "CrashLoopBackOff", 3, "app", hour, time.Duration(i+1)*minute)
		p.Sev = SevWaiting
		pods = append(pods, p)
	}
	// Ranks below the crashing pods, so the cap drops it.
	web := problem("shop", "web-6c9f7d8b5-xk2lp", "Pending", 0, "", 6*minute, 0)
	web.Sev = SevInfo
	pods = append(pods, web)
	s := Snapshot{
		Pods:      PodsPanel{Problems: pods},
		Workloads: WorkloadsPanel{Rows: []WorkloadRow{stuckDeployment("shop", "web")}},
	}
	got := suggest(s)
	if len(got) != maxSuggestions {
		t.Fatalf("got %d suggestions: %q", len(got), lines(got))
	}
	want := "kubectl describe deployment web -n shop    # why is the rollout stuck?"
	if !slices.Contains(lines(got), want) {
		t.Fatalf("a pod suggestion dropped at the cap still hid the workload: %q", lines(got))
	}
}
