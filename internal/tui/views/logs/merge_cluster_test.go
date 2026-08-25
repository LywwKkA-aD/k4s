//go:build cluster

package logs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

func TestMergeClusterSmoke(t *testing.T) {
	client, err := k8s.LoadFromKubeconfig("")
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pods, err := client.ListPods(ctx, "app")
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods) < 2 {
		t.Skipf("need >=2 pods in app namespace, got %d", len(pods))
	}

	names := []string{pods[0].Name, pods[1].Name}
	events := make(chan logEvent, 256)
	go runMergedStreams(ctx, client, "app", names, 100, "", events)

	counts := make(map[string]int)
	var first []string
	for ev := range events {
		if ev.Line != "" {
			counts[ev.Pod]++
			if len(first) < 30 {
				first = append(first, ev.Pod+": "+ev.Line)
			}
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
