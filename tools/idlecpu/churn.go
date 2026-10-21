package main

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// podSet maps "namespace/name" to the pod's resourceVersion.
type podSet map[string]string

// podLister lists every pod in the cluster.
type podLister func(ctx context.Context) (podSet, error)

// churn is how the pods differ between two lists. A pod that came and
// went between them is not seen, so the counts are a lower bound.
type churn struct {
	added, removed, updated int
	byNamespace             []nsCount // most changes first
}

type nsCount struct {
	ns string
	n  int
}

func (c churn) total() int { return c.added + c.removed + c.updated }

// podCheck is the outcome of listing pods before and after the window.
type podCheck struct {
	enabled bool // a lister was configured
	checked bool // both lists succeeded
	err     string
	churn   churn
}

// listTimeout bounds one kubectl call so a dead apiserver cannot hang the run.
const listTimeout = 20 * time.Second

const podColumns = "custom-columns=NS:.metadata.namespace,NAME:.metadata.name,RV:.metadata.resourceVersion"

// kubectlPodArgs lists pods in every namespace, one "ns name rv" row each.
func kubectlPodArgs(kubeconfig string) []string {
	args := []string{"get", "pods", "-A", "--no-headers", "-o", podColumns}
	if kubeconfig == "" {
		return args
	}
	return append([]string{"--kubeconfig", kubeconfig}, args...)
}

// kubectlPods lists pods with kubectl. The harness asks kubectl rather
// than client-go so the tools module stays free of the k8s dependency tree.
func kubectlPods(kubectl, kubeconfig string) podLister {
	return func(ctx context.Context) (podSet, error) {
		ctx, cancel := context.WithTimeout(ctx, listTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, kubectl, kubectlPodArgs(kubeconfig)...) //nolint:gosec // G204: kubectl is the operator's own --kubectl flag
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			first, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
			return nil, fmt.Errorf("kubectl get pods: %w: %s", err, first)
		}
		return parsePodList(out)
	}
}

// parsePodList reads kubectl's custom-columns rows.
func parsePodList(out []byte) (podSet, error) {
	pods := podSet{}
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("parse pod list row %q: want namespace, name and resourceVersion", strings.TrimSpace(line))
		}
		pods[f[0]+"/"+f[1]] = f[2]
	}
	return pods, nil
}

// diffPods counts pods added, removed and changed between two lists.
func diffPods(before, after podSet) churn {
	c := churn{}
	perNS := map[string]int{}
	for key, rv := range after {
		old, ok := before[key]
		switch {
		case !ok:
			c.added++
		case old != rv:
			c.updated++
		default:
			continue
		}
		perNS[namespaceOf(key)]++
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			c.removed++
			perNS[namespaceOf(key)]++
		}
	}
	c.byNamespace = sortedCounts(perNS)
	return c
}

func namespaceOf(key string) string {
	ns, _, _ := strings.Cut(key, "/")
	return ns
}

func sortedCounts(m map[string]int) []nsCount {
	if len(m) == 0 {
		return nil
	}
	out := make([]nsCount, 0, len(m))
	for ns, n := range m {
		out = append(out, nsCount{ns: ns, n: n})
	}
	slices.SortFunc(out, func(a, b nsCount) int {
		if a.n != b.n {
			return cmp.Compare(b.n, a.n)
		}
		return cmp.Compare(a.ns, b.ns)
	})
	return out
}

// compareSnapshots turns the two pod lists, or why one failed, into a podCheck.
func compareSnapshots(before podSet, beforeErr error, after podSet, afterErr error) podCheck {
	switch {
	case beforeErr != nil:
		return podCheck{enabled: true, err: "before the window: " + beforeErr.Error()}
	case afterErr != nil:
		return podCheck{enabled: true, err: "after the window: " + afterErr.Error()}
	}
	return podCheck{enabled: true, checked: true, churn: diffPods(before, after)}
}
