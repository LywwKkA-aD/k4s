//go:build cluster

package logs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// clusterKubeconfig names the cluster the cluster-tagged tests read. The
// package's TestMain points KUBECONFIG at a fake, so it has its own
// variable:
//
//	K4S_CLUSTER_KUBECONFIG=$PWD/.kube/config go test -tags cluster ./internal/tui/views/logs/
const clusterKubeconfig = "K4S_CLUSTER_KUBECONFIG"

func clusterNamespace() string {
	if ns := os.Getenv("K4S_CLUSTER_NAMESPACE"); ns != "" {
		return ns
	}
	return "k4s-demo"
}

func TestMergeClusterSmoke(t *testing.T) {
	client, err := k8s.LoadFromKubeconfig(os.Getenv(clusterKubeconfig))
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ns := clusterNamespace()
	pods, err := client.ListPods(ctx, ns)
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	var names []string
	for _, p := range pods {
		if p.Status == "Running" && len(names) < 2 {
			names = append(names, p.Name)
		}
	}
	if len(names) < 2 {
		t.Skipf("need >=2 running pods in %s, got %d", ns, len(names))
	}

	s := startSession(async.New(), client, ns, defaultCrashLog(), pumpMerged, names, 100)
	defer s.stop()
	for i, name := range names {
		s.attach(attachReq{pod: name, attempt: uint64(i + 1), opts: k8s.LogOptions{Tail: 100}})
	}

	counts := make(map[string]int)
	var first []string
	deadline := time.After(8 * time.Second)
read:
	for {
		select {
		case ev := <-s.events:
			if ev.Line != "" {
				counts[ev.Pod]++
				if len(first) < 30 {
					first = append(first, ev.Pod+": "+ev.Line)
				}
			}
		case <-deadline:
			break read
		}
	}

	for _, n := range names {
		fmt.Printf("pod %s: %d lines\n", n, counts[n])
	}
	fmt.Println("--- first 30 merged lines ---")
	for _, l := range first {
		fmt.Println(l)
	}

	if counts[names[0]] == 0 || counts[names[1]] == 0 {
		t.Logf("one pod produced no lines; counts=%v", counts)
	}
}
