//go:build cluster

package logs_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// TestSeedInitCrashloopIsReachable reads the seed pod of
// deploy/seed/70-init-crashloop.yaml on `make demo`, read-only. TestMain
// points KUBECONFIG at a fake, so the cluster comes in its own variable:
//
//	K4S_CLUSTER_KUBECONFIG=$PWD/.kube/config go test -tags cluster -run SeedInitCrashloop ./internal/tui/views/logs/
func TestSeedInitCrashloopIsReachable(t *testing.T) {
	client, err := k8s.LoadFromKubeconfig(os.Getenv("K4S_CLUSTER_KUBECONFIG"))
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := client.PodContainers(ctx, "k4s-broken", "init-crashloop")
	if err != nil {
		t.Skipf("seed pod not there: %v", err)
	}
	t.Logf("containers %v, init %v", cs.Names(), cs.InitNames())
	if !strings.Contains(strings.Join(cs.InitNames(), ","), "migrate") {
		t.Fatalf("migrate is not listed as an init container: %+v", cs)
	}

	driver := tuitest.Driver{CmdTimeout: 3 * time.Second}
	m := logs.New(client, "k4s-broken", []string{"init-crashloop"}, 20, "migrate")
	t.Cleanup(func() { _ = m.Close() })
	m, _ = tuitest.DriveWith(t, driver, m, tuitest.Resize(140, 30))
	m, _ = tuitest.InitWith(t, driver, m)
	screen := tuitest.Screen(m)
	t.Logf("current:\n%s\n%s", m.Command().String(), screen)
	if !strings.Contains(screen, "migration") {
		t.Errorf("the init container's log is not on screen")
	}

	m, out := tuitest.DriveWith(t, driver, m, tuitest.Press("p"))
	if st, ok := tuitest.Find[views.StatusMsg](out); ok {
		t.Logf("p said: %+v", st)
	}
	t.Logf("after p (%s):\n%s\n%s", m.Title(), m.Command().String(), tuitest.Screen(m))
}

// TestPreviousOfAPodThatNeverRestarted asks the cluster for the previous
// instance of a pod in k4s-demo with no restarts.
func TestPreviousOfAPodThatNeverRestarted(t *testing.T) {
	client, err := k8s.LoadFromKubeconfig(os.Getenv("K4S_CLUSTER_KUBECONFIG"))
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pods, err := client.ListPods(ctx, "k4s-demo")
	if err != nil {
		t.Skipf("demo namespace not there: %v", err)
	}
	name := ""
	for _, p := range pods {
		if p.Restarts == 0 {
			name = p.Name
			break
		}
	}
	if name == "" {
		t.Skip("every demo pod has restarted")
	}

	driver := tuitest.Driver{CmdTimeout: 3 * time.Second}
	m := logs.New(client, "k4s-demo", []string{name}, 5, "")
	t.Cleanup(func() { _ = m.Close() })
	m, _ = tuitest.DriveWith(t, driver, m, tuitest.Resize(140, 10))
	m, out := tuitest.DriveWith(t, driver, m, tuitest.Press("p"))
	st, ok := tuitest.Find[views.StatusMsg](out)
	t.Logf("pod %s, p said %+v, title %q", name, st, m.Title())
	if !ok || !strings.Contains(st.Text, "has no previous instance") || strings.Contains(m.Title(), "previous") {
		t.Errorf("want the classified status and the current stream kept")
	}
}
