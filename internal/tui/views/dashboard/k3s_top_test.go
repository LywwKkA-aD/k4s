package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestK3sPanel draws panel 6 as the wide mockup: the bundled components
// by name with their kind and readiness, ServiceLB, the helm charts and
// the etcd snapshot. A missing component is off, dim and never red; a
// denied helmcharts list hides its row.
func TestK3sPanel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		change    func(*dash.K3sPanel)
		want, not []string
	}{
		{
			name: "stock k3s",
			want: []string{
				"6 k3s ├", "bundled components",
				"● coredns                 deploy  1/1",
				"● traefik                 deploy  1/1",
				"● local-path-provisioner  deploy  1/1",
				"○ servicelb               off · 1 LoadBalancer <pending>",
				"● helm charts             traefik, traefik-crd · 0 failed",
				"◆ etcd snapshot           4h ago on k3s-cp-1",
			},
		},
		{
			name: "a component disabled and one not ready",
			change: func(k *dash.K3sPanel) {
				k.Components[1] = dash.Component{Key: dash.ObjectKey{Kind: "deployment", Namespace: "kube-system", Name: "traefik"}}
				k.Components[0].Ready, k.Components[0].Sev = 0, dash.SevFailing
			},
			want: []string{"○ traefik                 off", "▲ coredns                 deploy  0/1"},
			not:  []string{"▲ traefik"},
		},
		{
			name: "servicelb on, a chart failed, snapshots only reconciled",
			change: func(k *dash.K3sPanel) {
				k.ServiceLB = dash.ServiceLBRow{State: dash.StateOK, Ready: 2, Total: 2, Cmd: k.ServiceLB.Cmd}
				k.Helm.Failed = 1
				k.Etcd.Reconciled = true
			},
			want: []string{
				"● servicelb               2/2 svclb ready",
				"▲ helm charts             traefik, traefik-crd · 1 failed",
				"◆ snapshots reconciled    4h ago on k3s-cp-1",
			},
		},
		{
			name: "helm charts denied, no etcd, no LoadBalancers",
			change: func(k *dash.K3sPanel) {
				k.Helm = dash.HelmRow{Avail: denied("auth", "can-i", "list", "helmcharts.helm.cattle.io", "-A")}
				k.Etcd = dash.EtcdRow{}
				k.ServiceLB = dash.ServiceLBRow{State: dash.StateUnknown}
			},
			want: []string{"● coredns"},
			not:  []string{"helm charts", "etcd snapshot", "servicelb"},
		},
		{
			name:   "no snapshot yet",
			change: func(k *dash.K3sPanel) { k.Etcd.Known = false },
			want:   []string{"○ etcd snapshot           none yet"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := func() dash.Snapshot {
				s := busyK3s()
				if tt.change != nil {
					tt.change(&s.K3s)
				}
				return s
			}
			screen := tuitest.Screen(mount(t, newFeed(base), wideW, wideH))
			requireSize(t, screen, wideW, wideH)
			requireAll(t, screen, tt.want, tt.not)
		})
	}
}

// TestK3sRows: every line of panel 6 is a cursor stop with its command;
// Enter describes a component deployment and says y copies elsewhere.
func TestK3sRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		downs int
		cmd   string
		want  any
	}{
		{downs: 0, cmd: "kubectl get deploy,ds -n kube-system"},
		{downs: 1, cmd: "kubectl describe deployment coredns -n kube-system", want: views.DescribeRequestMsg{Kind: "deployment", Namespace: "kube-system", Name: "coredns"}},
		{downs: 5, cmd: "kubectl get svc -A --field-selector spec.type=LoadBalancer", want: views.OpenViewMsg{Name: "services"}},
		{downs: 6, cmd: "≈ kubectl get helmcharts -A"},
		{downs: 7, cmd: "kubectl get etcdsnapshotfiles"},
	}
	for _, tt := range tests {
		keys := append([]string{"6"}, strings.Fields(strings.Repeat("down ", tt.downs))...)
		m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), keys...)
		if got := command(m); got != tt.cmd {
			t.Errorf("6 and %d down: command %q, want %q", tt.downs, got, tt.cmd)
		}
		if tt.downs == 0 {
			continue
		}
		_, out := press(t, m, "enter")
		if tt.want != nil && !tuitest.Contains(out, tt.want) {
			t.Errorf("enter on row %d:\n%s", tt.downs, tuitest.Describe(out))
		}
		if msg, ok := tuitest.Find[views.StatusMsg](out); tt.want == nil && (!ok || !strings.Contains(msg.Text, "y copies")) {
			t.Errorf("enter on row %d:\n%s", tt.downs, tuitest.Describe(out))
		}
	}
}

// TestAnOffComponentPointsAtThePanel: a component k3s runs without has no
// object to describe; its row shows the panel's command, which lists what
// kube-system does run, and Enter describes nothing.
func TestAnOffComponentPointsAtThePanel(t *testing.T) {
	t.Parallel()

	base := func() dash.Snapshot {
		s := busyK3s()
		k := dash.ObjectKey{Kind: "deployment", Namespace: "kube-system", Name: "traefik"}
		s.K3s.Components[1] = dash.Component{Key: k, Cmd: kc("describe", "deployment", "traefik", "-n", "kube-system")}
		return s
	}
	m, _ := press(t, mount(t, newFeed(base), wideW, wideH), "6", "down", "down")
	if got := command(m); got != "kubectl get deploy,ds -n kube-system" {
		t.Fatalf("command on an off component = %q", got)
	}
	_, out := press(t, m, "enter")
	if _, ok := tuitest.Find[views.DescribeRequestMsg](out); ok {
		t.Fatalf("enter described an off component:\n%s", tuitest.Describe(out))
	}
}

// TestTopPodsPanel draws panel 7: the top pods in kubectl units with a bar
// relative to the busiest; s sorts by memory and back, and the footer's
// --sort-by, the border note and the hint follow.
func TestTopPodsPanel(t *testing.T) {
	t.Parallel()

	m := mount(t, newFeed(busyK3s), wideW, wideH)
	requireAll(t, tuitest.Screen(m), []string{
		"7 top pods ├", "by cpu",
		"NAMESPACE/POD", "CPU(cores)", "MEMORY",
		"◆ monitoring/prometheus-k8s-0", "1420m", "3172Mi",
		"◆ kube-system/coredns-ccb96694c-8xq2z",
		"bar relative to the top pod · 76 pods report · s memory",
	}, nil)
	m, _ = press(t, m, "7")
	if got := command(m); got != "kubectl top pods -A --sort-by=cpu" {
		t.Fatalf("panel 7 command = %q", got)
	}
	if got := hint(m, "s"); got != "sort by memory" {
		t.Fatalf("s hint = %q", got)
	}
	m, _ = press(t, m, "s")
	screen := tuitest.Screen(m)
	requireAll(t, screen, []string{"by memory", "s cpu"}, []string{"by cpu"})
	if got := command(m); got != "kubectl top pods -A --sort-by=memory" {
		t.Fatalf("sorted by memory: command = %q", got)
	}
	if i, j := strings.Index(screen, "loki-write-0"), strings.Index(screen, "payments-api-5f7b8c9d4"); i < 0 || j < 0 || i > j {
		t.Fatalf("by memory loki-write-0 should come before payments-api:\n%s", screen)
	}
	m, _ = press(t, m, "down")
	if got := command(m); got != "kubectl describe pod prometheus-k8s-0 -n monitoring" {
		t.Fatalf("first row by memory = %q", got)
	}
	_, out := press(t, m, "enter")
	if !tuitest.Contains(out, views.DescribeRequestMsg{Kind: "pod", Namespace: "monitoring", Name: "prometheus-k8s-0"}) {
		t.Fatalf("enter on a top pod:\n%s", tuitest.Describe(out))
	}
	_, out = press(t, m, "l")
	if _, ok := tuitest.Find[views.TailPromptRequestMsg](out); !ok {
		t.Fatalf("l on a top pod:\n%s", tuitest.Describe(out))
	}
	m, _ = press(t, m, "s")
	if got := command(m); got != "kubectl describe pod prometheus-k8s-0 -n monitoring" {
		t.Fatalf("s keeps the cursor on its pod: %q", got)
	}
}

// TestSortKeyOnlyOnPanel7: s does nothing elsewhere and is not hinted.
func TestSortKeyOnlyOnPanel7(t *testing.T) {
	t.Parallel()

	m, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3", "s")
	if len(out) != 0 || hint(m, "s") != "" {
		t.Fatalf("s on panel 3: hint %q, out:\n%s", hint(m, "s"), tuitest.Describe(out))
	}
	if !strings.Contains(tuitest.Screen(m), "by cpu") {
		t.Fatal("s on panel 3 sorted panel 7")
	}
}

// TestTopPodsDenied: a denied pod metrics list says so with its probe.
func TestTopPodsDenied(t *testing.T) {
	t.Parallel()

	base := func() dash.Snapshot {
		s := busyK3s()
		s.TopPods = dash.TopPodsPanel{Avail: denied("auth", "can-i", "list", "pods.metrics.k8s.io", "-A"), CmdCPU: s.TopPods.CmdCPU}
		return s
	}
	requireAll(t, tuitest.Screen(mount(t, newFeed(base), wideW, wideH)),
		[]string{"— pod metrics forbidden", "kubectl auth can-i list pods.metrics.k8s.io -A"}, []string{"1420m"})
}

// TestK3sAndTopPodsSummaries: the medium strip lists the components, the
// accordion's lines count them and name the busiest pods by workload.
func TestK3sAndTopPodsSummaries(t *testing.T) {
	t.Parallel()

	medium := tuitest.Screen(mount(t, newFeed(busyK3s), 120, 34))
	requireAll(t, medium, []string{"6 k3s        ● coredns 1/1  ● traefik 1/1  ● metrics-server 1/1  ● local-path 1/1  ○ servicelb off"}, nil)
	compact := tuitest.Screen(mount(t, newFeed(busyK3s), 80, 18))
	requireAll(t, compact, []string{
		"6 k3s        ● 4/4 components · ○ servicelb off · ◆ etcd 4h ago",
		"7 top pods   ◆ prometheus-k8s-0 1420m · payments-api 910m · ingest 770m",
	}, nil)
}
