package dashboard_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// screenOf is the busy fixture changed by change, at the wide size.
func screenOf(t *testing.T, change func(*dash.Snapshot), keys ...string) string {
	t.Helper()
	base := func() dash.Snapshot {
		s := busyK3s()
		if change != nil {
			change(&s)
		}
		return s
	}
	m, _ := press(t, mount(t, newFeed(base), wideW, wideH), keys...)
	screen := tuitest.Screen(m)
	requireSize(t, screen, wideW, wideH)
	return screen
}

func requireAll(t *testing.T, screen string, want, not []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	for _, n := range not {
		if strings.Contains(screen, n) {
			t.Errorf("screen shows %q:\n%s", n, screen)
		}
	}
}

// TestWorkloadsPanel draws panel 4 as the mockup: a ready bar per kind,
// the jobs and cron lines, then the rows that need a look.
func TestWorkloadsPanel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		change    func(*dash.Snapshot)
		want, not []string
	}{
		{
			name: "busy",
			want: []string{
				"┤ 4 workloads ├", "61/65 ready",
				"deploy  ███", "49/52", "sts     ███", "7/8", "ds      ███", "5/5",
				"jobs    ▲ 1 failed in 1h · 2 active",
				"cron    14 · ○ 1 suspended · last schedule 3m",
				"KIND    NAMESPACE/NAME            READY  UP-TO-DATE  AVAIL",
				"▲ deploy  shop/payments-worker      0/2    2           0",
				"▲ sts     monitoring/loki-backend   0/1",
				"▲ job     ci/nightly-e2e-28741000   failed 41m",
				"○ cron    growlytics/report-weekly  suspended",
				"5 need attention",
			},
		},
		{
			name: "all ready",
			change: func(s *dash.Snapshot) {
				s.Workloads.Rows = nil
				s.Workloads.Deploy.Ready, s.Workloads.STS.Ready = 52, 8
				s.Workloads.Jobs.FailedRecent, s.Workloads.Cron.Suspended = 0, 0
			},
			want: []string{"● all 65 ready", "jobs    ● none failed in 1h · 2 active", "cron    14 · last schedule 3m", "65/65 ready"},
			not:  []string{"KIND", "5 need attention"},
		},
		{
			name: "a partly ready workload",
			change: func(s *dash.Snapshot) {
				s.Workloads.Rows = []dash.WorkloadRow{workload("daemonset", "kube-system", "svclb-traefik", 2, 3, 0, 2, dash.SevWaiting)}
				s.Workloads.DS.Ready = 4
			},
			want: []string{"△ ds      kube-system/svclb-traefik  2/3", "1 need attention"},
		},
		{
			name: "a denied kind says so on its bar line only",
			change: func(s *dash.Snapshot) {
				s.Workloads.STS = dash.KindSummary{Avail: denied("auth", "can-i", "list", "statefulsets", "-A")}
				s.Workloads.Rows = s.Workloads.Rows[:3]
			},
			want: []string{"sts     — statefulsets forbidden", "deploy  ███", "54/57 ready"},
			not:  []string{"0/0"},
		},
		{
			name: "no cronjobs, jobs denied",
			change: func(s *dash.Snapshot) {
				s.Workloads.Cron = dash.CronSummary{Avail: ok()}
				s.Workloads.Jobs = dash.JobsSummary{Avail: denied("auth", "can-i", "list", "jobs", "-A")}
				s.Workloads.Rows = s.Workloads.Rows[:4]
			},
			want: []string{"cron    ○ no cronjobs", "jobs    — jobs forbidden"},
		},
		{
			name: "a kind with none",
			change: func(s *dash.Snapshot) {
				s.Workloads.STS = dash.KindSummary{Avail: ok()}
				s.Workloads.Rows = s.Workloads.Rows[:3]
			},
			want: []string{"sts     ○ none"},
		},
		{
			name: "a row the lookup has not placed yet",
			change: func(s *dash.Snapshot) {
				s.Workloads.Rows[0].Resolved, s.Workloads.Rows[0].Key.Namespace = false, ""
			},
			want: []string{"▲ deploy  —/payments-worker"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireAll(t, screenOf(t, tt.change), tt.want, tt.not)
		})
	}
}

// TestEventsPanel draws panel 5: the last hour as a strip, then the
// newest warnings. A warning moves the verdict at most to △, and only
// when it counts (recent, on an object no other panel flags).
func TestEventsPanel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		change    func(*dash.Snapshot)
		want, not []string
	}{
		{
			name: "busy",
			want: []string{
				"┤ 5 events ├", "type=Warning · 1h",
				"≈ last hour", "10 warnings · 7 reasons · 9 objects",
				"LAST SEEN  REASON", "OBJECT", "COUNT  MESSAGE",
				"◇ <1m        BackOff               pod/loki-backend-0 · monitoring",
				"×112   Back-off restarting failed container ingester",
				"△ 4m         FailedMount           pod/backup-7d9f8c6b5-q2w4e · growlytics",
				"◇ 41m        BackoffLimitExceeded  job/nightly-e2e-28741000 · ci",
				"newest first · 10 of 10",
			},
			not: []string{"▲ <1m"},
		},
		{
			name:   "a capped list",
			change: func(s *dash.Snapshot) { s.Events.Total, s.Events.Truncated = 500, true },
			want:   []string{"≥500 warnings", "newest first · 10 of ≥500"},
		},
		{
			name:   "no warnings",
			change: func(s *dash.Snapshot) { s.Events = dash.EventsPanel{Avail: ok(), Cmd: s.Events.Cmd} },
			want:   []string{"● no warnings in the last hour", "events live about an hour"},
			not:    []string{"LAST SEEN"},
		},
		{
			name: "denied",
			change: func(s *dash.Snapshot) {
				s.Events = dash.EventsPanel{Avail: denied("auth", "can-i", "list", "events", "-A"), Cmd: s.Events.Cmd}
			},
			want: []string{"— events forbidden", "kubectl auth can-i list events -A"},
			not:  []string{"0 warnings"},
		},
		{
			name: "a cluster-scoped object",
			change: func(s *dash.Snapshot) {
				s.Events.Rows = []dash.EventRow{{
					Object: dash.ObjectKey{Kind: "node", Name: "k3s-agent-1"}, Reason: "FreeDiskSpaceFailed",
					Message: "failed to garbage collect required amount of images", Count: 2, Counted: true,
					Cmd: kc("describe", "node", "k3s-agent-1"),
				}}
			},
			want: []string{"△ <1m        FreeDiskSpaceFailed  node/k3s-agent-1  ×2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requireAll(t, screenOf(t, tt.change), tt.want, tt.not)
		})
	}
}

// TestPanelRowsCarryTheirCommands walks the rows of panels 4 and 5: each
// stands for its object's describe, and the panel for its list.
func TestPanelRowsCarryTheirCommands(t *testing.T) {
	t.Parallel()

	const firstDeploy = "kubectl describe deployment payments-worker -n shop"
	tests := []struct {
		keys []string
		w, h int
		want string
	}{
		{keys: []string{"4"}, want: "kubectl get deploy,sts,ds -A"},
		{keys: []string{"4", "down"}, want: "kubectl get jobs -A --field-selector status.successful=0"},
		{keys: []string{"4", "down", "down"}, want: "kubectl get cronjobs -A"},
		{keys: []string{"4", "down", "down", "down"}, want: firstDeploy},
		{keys: []string{"4", "down", "down", "down", "down", "down", "down"}, want: "kubectl describe statefulset loki-backend -n monitoring"},
		{keys: []string{"4", "down", "down", "down", "down", "down", "down", "down", "down", "down"}, want: "kubectl describe cronjob report-weekly -n growlytics"},
		// Folded into one line, the jobs and cron lines are no stops.
		{keys: []string{"4", "down"}, w: 120, h: 34, want: firstDeploy},
		{keys: []string{"5"}, want: "kubectl events -A --types=Warning"},
		{keys: []string{"5", "down"}, want: "kubectl describe pod loki-backend-0 -n monitoring"},
		{keys: []string{"5", "down", "down", "down", "down", "down", "down", "down", "down", "down"}, want: "kubectl describe job nightly-e2e-28741000 -n ci"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %dx%d", strings.Join(tt.keys, " "), tt.w, tt.h), func(t *testing.T) {
			t.Parallel()
			w, h := wideW, wideH
			if tt.w > 0 {
				w, h = tt.w, tt.h
			}
			m, _ := press(t, mount(t, newFeed(busyK3s), w, h), tt.keys...)
			if got := command(m); got != tt.want {
				t.Fatalf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestJobsAndCronLinesTakeTheCursor: the cursor stands on the jobs and
// cron lines like on any row.
func TestJobsAndCronLinesTakeTheCursor(t *testing.T) {
	t.Parallel()

	screen := screenOf(t, nil, "4", "down", "down")
	requireAll(t, screen, []string{"┃›cron    14 · ○ 1 suspended · last schedule 3m", "2/8 · 5 need attention"}, nil)
}

// TestEnterOnPanelRows opens describe where k4s has the view (pods,
// deployments and services) and says so elsewhere.
func TestEnterOnPanelRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		want views.DescribeRequestMsg
		hint string
	}{
		{name: "deployment row", keys: []string{"4", "down", "down", "down"}, want: views.DescribeRequestMsg{Kind: "deployment", Namespace: "shop", Name: "payments-worker"}, hint: "describe"},
		{name: "pod event", keys: []string{"5", "down"}, want: views.DescribeRequestMsg{Kind: "pod", Namespace: "monitoring", Name: "loki-backend-0"}, hint: "describe"},
		{name: "statefulset row", keys: []string{"4", "down", "down", "down", "down", "down", "down"}},
		{name: "jobs line", keys: []string{"4", "down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), tt.keys...)
			if got := hint(m, "enter"); got != tt.hint {
				t.Fatalf("enter hint = %q, want %q", got, tt.hint)
			}
			_, out := press(t, m, "enter")
			if tt.want.Kind == "" {
				if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "no k4s view") {
					t.Fatalf("enter without a view:\n%s", tuitest.Describe(out))
				}
				return
			}
			if !tuitest.Contains(out, tt.want) {
				t.Fatalf("enter:\n%s", tuitest.Describe(out))
			}
		})
	}
}
