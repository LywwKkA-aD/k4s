package k8s

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// clusterKubeconfigEnv names a kubeconfig for a live cluster. When it is
// set, TestPodColumnsMatchKubectl compares ListPods with kubectl; it only
// reads from the cluster.
const clusterKubeconfigEnv = "K4S_CLUSTER_KUBECONFIG"

func TestPodColumnsMatchKubectl(t *testing.T) {
	t.Parallel()

	path := os.Getenv(clusterKubeconfigEnv)
	if path == "" {
		t.Skip(clusterKubeconfigEnv + " not set")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "--all-namespaces", "--no-headers")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl get pods: %v", err)
	}
	client, err := LoadFromKubeconfig(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	pods, err := client.ListPods(ctx, "")
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	ours := make(map[string]Pod, len(pods))
	for _, p := range pods {
		ours[p.Namespace+"/"+p.Name] = p
	}

	compared := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		row, ok := parseKubectlPodRow(line)
		if !ok {
			continue
		}
		// Pods come and go between the two lists (CronJobs, rollouts).
		p, found := ours[row.key]
		if !found {
			continue
		}
		compared++
		if p.Ready != row.ready || p.Status != row.status {
			t.Errorf("%s: k4s READY/STATUS %s %s, kubectl %s %s", row.key, p.Ready, p.Status, row.ready, row.status)
		}
		// The "(96s ago)" part moves between the two calls; the count and
		// whether a time is shown must not.
		if restartCount(p.RestartsText) != restartCount(row.restarts) || hasAgo(p.RestartsText) != hasAgo(row.restarts) {
			t.Errorf("%s: k4s RESTARTS %q, kubectl %q", row.key, p.RestartsText, row.restarts)
		}
	}
	if compared == 0 {
		t.Fatal("no pod appeared in both lists")
	}
	t.Logf("compared %d pods", compared)
}

type kubectlPodRow struct {
	key, ready, status, restarts string
}

// parseKubectlPodRow splits "ns name 1/1 Running 3 (5m ago) 12h".
func parseKubectlPodRow(line string) (kubectlPodRow, bool) {
	f := strings.Fields(line)
	if len(f) < 6 {
		return kubectlPodRow{}, false
	}
	return kubectlPodRow{
		key:      f[0] + "/" + f[1],
		ready:    f[2],
		status:   f[3],
		restarts: strings.Join(f[4:len(f)-1], " "),
	}, true
}

func restartCount(cell string) string {
	count, _, _ := strings.Cut(cell, " ")
	return count
}

func hasAgo(cell string) bool { return strings.HasSuffix(cell, " ago)") }
