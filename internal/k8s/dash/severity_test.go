package dash

import (
	"fmt"
	"testing"
	"time"
)

func TestClassAndSeverityOfPodStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status    string
		ready, of int
		class     Class
		sev       Severity
	}{
		{"Running", 1, 1, ClassOK, SevOK},
		{"Running", 1, 2, ClassNotReady, SevWaiting},
		{"Completed", 0, 1, ClassDone, SevOK},
		{"CrashLoopBackOff", 0, 1, ClassCrash, SevFailing},
		{"Error", 0, 1, ClassCrash, SevFailing},
		{"OOMKilled", 0, 1, ClassCrash, SevFailing},
		{"Init:CrashLoopBackOff", 0, 1, ClassCrash, SevFailing},
		{"Init:Error", 0, 1, ClassCrash, SevFailing},
		{"ImagePullBackOff", 0, 1, ClassPull, SevFailing},
		{"ErrImagePull", 0, 1, ClassPull, SevFailing},
		{"Init:ImagePullBackOff", 0, 1, ClassPull, SevFailing},
		{"CreateContainerConfigError", 0, 1, ClassConfig, SevFailing},
		{"Pending", 0, 1, ClassPending, SevWaiting},
		{"ContainerCreating", 0, 1, ClassPending, SevWaiting},
		{"PodInitializing", 0, 1, ClassPending, SevWaiting},
		{"Init:0/2", 0, 1, ClassPending, SevWaiting},
		{"Evicted", 0, 1, ClassEvicted, SevMinor},
		{"Terminating", 1, 1, ClassOther, SevOK},
		{"Unknown", 0, 1, ClassOther, SevOK},
	}
	for _, tc := range cases {
		c := classOf(tc.status, tc.ready, tc.of, true)
		if c != tc.class || severityOf(c) != tc.sev {
			t.Errorf("%s %d/%d: class %v sev %v, want %v %v", tc.status, tc.ready, tc.of, c, severityOf(c), tc.class, tc.sev)
		}
	}
	if classOf("Running", 0, 1, false) != ClassOK {
		t.Error("an unreadable READY cell must not mark a pod not ready")
	}
}

func TestWorkloadAndMeterSeverity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		ready, desired int
		want           Severity
	}{{0, 2, SevFailing}, {1, 2, SevWaiting}, {2, 2, SevOK}, {0, 0, SevOK}} {
		if got := workloadSeverity(tc.ready, tc.desired); got != tc.want {
			t.Errorf("%d/%d = %v, want %v", tc.ready, tc.desired, got, tc.want)
		}
	}
	for pct, want := range map[int]Severity{84: SevOK, 85: SevWaiting, 99: SevWaiting} {
		if got := meterSeverity(pct); got != want {
			t.Errorf("meter %d%% = %v, want %v", pct, got, want)
		}
	}
}

// A hot meter and its verdict chip must carry the same severity, so the
// glyph agrees under NO_COLOR.
func TestHotMeterGlyphMatchesItsChip(t *testing.T) {
	t.Parallel()

	s := refresh(t, newAPIServer(t, busyK3s(fixedNow)), Scope{})
	found := false
	for _, r := range s.Nodes.Rows {
		for _, m := range []struct {
			name  string
			meter Meter
		}{{"cpu", r.CPU}, {"mem", r.Memory}} {
			if m.meter.Sev == SevOK {
				continue
			}
			text := fmt.Sprintf("%s %s %d%%", r.Key.Name, m.name, m.meter.Percent)
			for _, c := range s.Verdict.Chips {
				if c.Text == text {
					found = true
					if c.Sev != m.meter.Sev || r.Sev != m.meter.Sev {
						t.Errorf("%s: meter %v, chip %v, row %v", text, m.meter.Sev, c.Sev, r.Sev)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("the busy fixture has no hot-meter chip")
	}
}

func TestWarningRule(t *testing.T) {
	t.Parallel()

	ready := podRow{name: "web", status: "Running", ready: 1, readyOf: 1, readyOK: true}
	notReady := podRow{name: "api", status: "Running", ready: 0, readyOf: 1, readyOK: true}
	f := flagged{
		objects:   map[ObjectKey]bool{{Kind: "pod", Namespace: "shop", Name: "crash"}: true},
		names:     map[string]bool{"pod/unplaced": true},
		deploys:   []string{"payments"},
		pods:      map[string]podRow{"web": ready, "api": notReady, "crash": {name: "crash"}, "unplaced": {name: "unplaced"}},
		nodes:     map[string]bool{"node-1": true},
		workloads: map[string]bool{"deployment/web": true},
	}
	pod := func(name string) ObjectKey { return ObjectKey{Kind: "pod", Namespace: "shop", Name: name} }
	cases := []struct {
		name   string
		reason string
		key    ObjectKey
		age    time.Duration
		f      flagged
		want   bool
	}{
		{"recent warning on a healthy pod counts", "FailedMount", pod("web"), 5 * minute, f, true},
		{"an old warning does not", "FailedMount", pod("web"), 11 * minute, f, false},
		{"a routine kubelet warning does not", "DNSConfigForming", pod("web"), minute, f, false},
		{"a recovered readiness probe does not", "Unhealthy", pod("web"), minute, f, false},
		{"a failing probe on a pod still not ready does", "Unhealthy", pod("api"), minute, f, true},
		{"a pod already flagged is not counted twice", "BackOff", pod("crash"), minute, f, false},
		{"a pod flagged before placement is not either", "BackOff", pod("unplaced"), minute, f, false},
		{"a pod that is gone does not", "Unhealthy", pod("deleted"), minute, f, false},
		{"without a pods list, assume it exists", "FailedMount", pod("deleted"), minute, flagged{}, true},
		{"an existing node counts", "SystemOOM", ObjectKey{Kind: "node", Name: "node-1"}, minute, f, true},
		{"a gone node does not", "SystemOOM", ObjectKey{Kind: "node", Name: "node-9"}, minute, f, false},
		{"a ReplicaSet of a flagged deployment is that deployment", "FailedCreate", ObjectKey{Kind: "replicaset", Namespace: "shop", Name: "payments-6c9f7d8b5"}, minute, f, false},
		{"another ReplicaSet counts", "FailedCreate", ObjectKey{Kind: "replicaset", Namespace: "shop", Name: "web-6c9f7d8b5"}, minute, f, true},
		{"an existing deployment counts", "ProgressDeadlineExceeded", ObjectKey{Kind: "deployment", Namespace: "shop", Name: "web"}, minute, f, true},
		{"a gone deployment does not", "ProgressDeadlineExceeded", ObjectKey{Kind: "deployment", Namespace: "shop", Name: "old"}, minute, f, false},
		{"other kinds count", "ProvisioningFailed", ObjectKey{Kind: "persistentvolumeclaim", Namespace: "shop", Name: "data"}, minute, f, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := counts(eventInfo{reason: tc.reason}, tc.key, tc.age, tc.f); got != tc.want {
				t.Fatalf("counts = %v, want %v", got, tc.want)
			}
		})
	}
}
