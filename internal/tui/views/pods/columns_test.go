package pods_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// restarted is a pod whose only container restarted 3 times, last 3h ago.
// Hours keep "(3h ago)" stable however slow the test run.
func restarted(ns, name string) *corev1.Pod {
	p := tuitest.Pod(ns, name, nil, tuitest.ContainerApp)
	cs := &p.Status.ContainerStatuses[0]
	cs.RestartCount = 3
	cs.LastTerminationState.Terminated = &corev1.ContainerStateTerminated{
		FinishedAt: metav1.NewTime(time.Now().Add(-3 * time.Hour)),
	}
	return p
}

func TestColumnsShowWhatKubectlPrints(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{tuitest.Namespace(demo), restarted(demo, "api-0")}
	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(objs...)), demo)
	row := ""
	for _, l := range strings.Split(tuitest.Screen(m), "\n") {
		if strings.Contains(l, "api-0") {
			row = l
		}
	}
	for _, want := range []string{"1/1", "Running", "3 (3h ago)", "90m"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q lacks %q", row, want)
		}
	}
}

// TestTableFitsTheWidth: at 80 columns and wider no line of the table
// wraps, in one namespace and in all of them.
func TestTableFitsTheWidth(t *testing.T) {
	t.Parallel()

	long := "payments-worker-7d9c8b6f5-abcde"
	objs := []runtime.Object{
		tuitest.Namespace("cert-manager"),
		restarted("cert-manager", long),
		tuitest.Pod("cert-manager", "short", nil, tuitest.ContainerApp),
	}
	cases := []struct {
		namespace string
		width     int
	}{
		{namespace: "", width: 80},
		{namespace: "cert-manager", width: 80},
		{namespace: "", width: 120},
		{namespace: "cert-manager", width: 200},
	}
	for _, tc := range cases {
		t.Run(tc.namespace+"@"+strconv.Itoa(tc.width), func(t *testing.T) {
			t.Parallel()
			m := newView(tuitest.NewFakeClient(t, tuitest.WithObjects(objs...)), tc.namespace)
			m, _ = tuitest.Mount(t, m, tc.width, 20)
			screen := tuitest.Screen(m)
			for _, l := range strings.Split(screen, "\n") {
				if w := ansi.StringWidth(l); w > tc.width {
					t.Errorf("line is %d wide, over %d: %q", w, tc.width, l)
				}
			}
			if !strings.Contains(screen, "3 (3h ago)") {
				t.Errorf("RESTARTS is cut:\n%s", screen)
			}
			if tc.width >= 120 && !strings.Contains(screen, long) {
				t.Errorf("NAME does not widen with the screen:\n%s", screen)
			}
		})
	}
}
