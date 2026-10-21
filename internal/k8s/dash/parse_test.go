package dash

import (
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseReadyz(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		body   string
		ready  bool
		checks int
		failed []string
		etcd   State
	}{
		{"passing", readyOK, true, 8, nil, StateOK},
		{"failing etcd", "[+]ping ok\n[-]etcd failed: reason withheld\nreadyz check failed\n", false, 2, []string{"etcd"}, StateFailed},
		{"kine without an etcd check", "[+]ping ok\n[+]log ok\nreadyz check passed\n", true, 2, nil, StateUnknown},
		{"cut short by the 2 KiB error cap", "[+]ping ok\n[-]informer-sync failed: reason withheld\n[+]log o", false, 3, []string{"informer-sync"}, StateUnknown},
	}
	for _, tc := range cases {
		got := parseReadyz([]byte(tc.body))
		if got.ready != tc.ready || got.checks != tc.checks || !slices.Equal(got.failed, tc.failed) || got.etcd != tc.etcd {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
}

func TestVersionHelpers(t *testing.T) {
	t.Parallel()

	for git, want := range map[string]string{
		"v1.36.3+k3s1": "k3s", "v1.31.2+rke2r1": "rke2", "v1.35.4-eks-3025e55": "eks", "v1.33.5-gke.1080000": "gke", "v1.33.1": "",
	} {
		if got := distroOf(git); got != want {
			t.Errorf("distroOf(%q) = %q, want %q", git, got, want)
		}
	}
	for minor, want := range map[string]int{"31": 31, "31+": 31, "": 0, "x": 0} {
		if got := minorOf(minor); got != want {
			t.Errorf("minorOf(%q) = %d, want %d", minor, got, want)
		}
	}
}

func TestEventsHelpers(t *testing.T) {
	t.Parallel()

	t0 := fixedNow.Add(-time.Hour)
	series := corev1.Event{Series: &corev1.EventSeries{Count: 3, LastObservedTime: metav1.NewMicroTime(t0.Add(3 * time.Minute))},
		LastTimestamp: metav1.NewTime(t0.Add(time.Minute)), EventTime: metav1.NewMicroTime(t0)}
	legacy := corev1.Event{LastTimestamp: metav1.NewTime(t0.Add(time.Minute)), EventTime: metav1.NewMicroTime(t0)}
	modern := corev1.Event{EventTime: metav1.NewMicroTime(t0)}
	if !lastSeen(series).Equal(t0.Add(3*time.Minute)) || !lastSeen(legacy).Equal(t0.Add(time.Minute)) || !lastSeen(modern).Equal(t0) {
		t.Fatal("lastSeen does not prefer series.lastObservedTime, then lastTimestamp, then eventTime")
	}
	for path, want := range map[string]string{
		"spec.containers{worker}": "worker", "spec.initContainers{migrate}": "migrate", "spec.ephemeralContainers{debug}": "debug", "": "", "metadata": "",
	} {
		if got := containerOf(path); got != want {
			t.Errorf("containerOf(%q) = %q, want %q", path, got, want)
		}
	}
	normal := corev1.Event{Type: corev1.EventTypeNormal, Reason: "Pulled"}
	warn := corev1.Event{Type: corev1.EventTypeWarning, Reason: "BackOff"}
	if got := eventsFrom([]corev1.Event{normal, warn}); len(got) != 1 || got[0].count != 1 {
		t.Fatalf("eventsFrom = %+v", got)
	}
}

// Events sort client-side by series.lastObservedTime, then lastTimestamp,
// then eventTime, newest first, and fill the hour strip.
func TestDeriveEventsOrderAndStrip(t *testing.T) {
	t.Parallel()

	st := store{events: []eventInfo{
		{kind: "Pod", namespace: "a", name: "old", reason: "BackOff", last: fixedNow.Add(-50 * time.Minute)},
		{kind: "Pod", namespace: "a", name: "new", reason: "BackOff", last: fixedNow.Add(-30 * time.Second)},
		{kind: "Node", name: "n1", reason: "SystemOOM", last: fixedNow.Add(-3 * time.Minute)},
		{kind: "Pod", namespace: "a", name: "ancient", reason: "BackOff", last: fixedNow.Add(-2 * time.Hour)},
	}}
	var led Ledger
	led[Events].State = StateOK
	p := deriveEvents(st, led, Scope{}, flagged{}, fixedNow)
	order := make([]string, 0, len(p.Rows))
	for _, r := range p.Rows {
		order = append(order, r.Object.Name)
	}
	if !slices.Equal(order, []string{"new", "n1", "old", "ancient"}) {
		t.Fatalf("order = %v", order)
	}
	if p.LastHour[29] != 1 || p.LastHour[28] != 1 || p.LastHour[4] != 1 || p.Counted != 2 || p.Reasons != 2 || p.Objects != 4 {
		t.Fatalf("panel = %+v", p)
	}
	if got := p.Rows[1].Cmd.String(); got != "kubectl describe node n1" {
		t.Fatalf("node event command = %q", got)
	}
}

func TestMetricsAndTopPods(t *testing.T) {
	t.Parallel()

	if _, err := metricsFrom([]byte("{")); err == nil {
		t.Fatal("want a decode error")
	}
	rows, err := metricsFrom([]byte(`{"items":[{"metadata":{"name":"n1"},"usage":{"cpu":"1500m","memory":"2Gi"}},
	 {"metadata":{"name":"p","namespace":"a"},"containers":[{"usage":{"cpu":"250m","memory":"64Mi"}},{"usage":{"cpu":"bad","memory":"16Mi"}}]}]}`))
	if err != nil || len(rows) != 2 || rows[0].cpuMilli != 1500 || rows[1].cpuMilli != 250 || rows[1].memBytes != 80<<20 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if FormatCPU(1420) != "1420m" || FormatMemory(3172<<20) != "3172Mi" {
		t.Fatal("kubectl top units are wrong")
	}
	many := make([]usageRow, 0, 12)
	for i := range 12 {
		many = append(many, usageRow{name: string(rune('a' + i)), namespace: "x", cpuMilli: int64(i), memBytes: int64(12 - i)})
	}
	top := topBy(many, func(r usageRow) int64 { return r.cpuMilli })
	if len(top) != topPodRows || top[0].Key.Name != "l" || top[0].Cmd.String() != "kubectl describe pod l -n x" {
		t.Fatalf("top = %+v", top)
	}
}

func TestTableRowDecoders(t *testing.T) {
	t.Parallel()

	tb, err := decodeTable([]byte(`{"columnDefinitions":[{"name":"Name"},{"name":"Desired"},{"name":"Ready"},{"name":"Available"}],
	 "rows":[{"cells":["svclb-traefik",3,2,2]},{"cells":[7,1,1,1]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := dsRowsFrom(tb, "kube-system"); len(got) != 1 || got[0].desired != 3 || got[0].ready != 2 || got[0].namespace != "kube-system" {
		t.Fatalf("ds rows = %+v", got)
	}
	tb, _ = decodeTable([]byte(`{"columnDefinitions":[{"name":"Name"},{"name":"Node"},{"name":"Age"}],"rows":[{"cells":["s1","cp-1","4h"]},{"cells":["s2","cp-1","bad"]}]}`))
	if got := snapshotRowsFrom(tb, fixedNow); len(got) != 1 || got[0].node != "cp-1" || !got[0].created.Equal(fixedNow.Add(-4*time.Hour)) {
		t.Fatalf("snapshot rows = %+v", got)
	}
}

func TestMissingColumnsAreNotGuessed(t *testing.T) {
	t.Parallel()

	tb, _ := decodeTable([]byte(`{"columnDefinitions":[{"name":"Name"}],"rows":[{"cells":["traefik"]}]}`))
	if got := helmRowsFrom(tb); len(got) != 1 || got[0].job != "helm-install-traefik" {
		t.Fatalf("helm rows = %+v", got)
	}
	tb, _ = decodeTable([]byte(`{"columnDefinitions":[{"name":"Name"},{"name":"Status"}],"rows":[{"cells":["web"]}]}`))
	if got := missingColumns(tb, podColumns); !slices.Equal(got, []string{"Ready", "Restarts", "Age", "Node"}) {
		t.Fatalf("missing = %v", got)
	}
	if rows := podRowsFrom(tb, "", fixedNow); len(rows) != 1 || rows[0].readyOK || rows[0].node != "" || !rows[0].created.IsZero() {
		t.Fatalf("a pod row with missing columns guessed: %+v", rows)
	}
	for _, s := range []string{"1", "a/1", "1/b"} {
		if _, _, ok := parseReady(s); ok {
			t.Errorf("parseReady(%q) succeeded", s)
		}
	}
}
