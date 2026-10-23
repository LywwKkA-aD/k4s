//go:build cluster

package logs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestFollowAgainstTheCluster resolves the seed's log-spammer deployment,
// read-only, and lets a minute of re-resolves run against the API. Its
// streams are silent stand-ins, so the view is driven by the clock alone:
//
//	K4S_CLUSTER_KUBECONFIG=$PWD/.kube/config go test -tags cluster -run FollowAgainst ./internal/tui/views/logs/
func TestFollowAgainstTheCluster(t *testing.T) {
	client, err := k8s.LoadFromKubeconfig(os.Getenv(clusterKubeconfig))
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	ns := clusterNamespace()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target, err := client.ResolveLogTarget(ctx, k8s.LogKindDeployment, ns, "log-spammer")
	if err != nil || len(target.Pods) == 0 {
		t.Skipf("seed deployment not there: %v", err)
	}

	driver := tuitest.Driver{CmdTimeout: 3 * time.Second}
	clock := asynctest.NewClock(time.Now())
	src := &recorder{}
	m := New(client, ns, target.PodNames(), 5, "", WithTarget(target), WithClock(clock), withSource(src))
	t.Cleanup(func() { _ = m.Close() })
	m, _ = tuitest.DriveWith(t, driver, m, tuitest.Resize(160, 30))
	for range 6 {
		var out []any
		next, msgs := tuitest.DriveWith(t, driver, m, clock.Advance(followInterval)...)
		m = next
		for _, st := range tuitest.All[views.StatusMsg](msgs) {
			out = append(out, st)
		}
		if len(out) > 0 {
			t.Errorf("a re-resolve said %+v", out)
		}
	}
	t.Logf("%s\n%s", m.Title(), m.Command().String())
	if !strings.Contains(m.Command().String(), "pod-template-hash in (") {
		t.Errorf("footer = %s", m.Command())
	}
	if want := fmt.Sprintf("deployment · %s · logs · %d pods", target.Name, len(target.Pods)); len(target.Pods) > 1 && !strings.HasPrefix(m.Title(), want) {
		t.Errorf("a steady minute changed the pods: title %q, want %q", m.Title(), want)
	}
	for _, pod := range target.PodNames() {
		if n := src.count(pod); n != 1 {
			t.Errorf("pod %s was streamed %d times", pod, n)
		}
	}
}
