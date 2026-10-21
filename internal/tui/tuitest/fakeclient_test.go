package tuitest_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func podNames(t *testing.T, c *k8s.Client, ns string) []string {
	t.Helper()
	pods, err := c.ListPods(context.Background(), ns)
	if err != nil {
		t.Fatalf("ListPods(%q): %v", ns, err)
	}
	out := make([]string, 0, len(pods))
	for _, p := range pods {
		out = append(out, p.Name)
	}
	return out
}

func TestFakeClientSeed(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	ctx := context.Background()
	if c.Context != tuitest.FakeContext {
		t.Errorf("Context = %q, want %q", c.Context, tuitest.FakeContext)
	}
	wantDemo := []string{tuitest.PodCrash, tuitest.PodMulti, tuitest.PodWeb1, tuitest.PodWeb2}
	if got := podNames(t, c, tuitest.NamespaceDemo); !slices.Equal(got, wantDemo) {
		t.Errorf("demo pods = %v, want %v (sorted)", got, wantDemo)
	}
	if got := podNames(t, c, ""); len(got) != 5 {
		t.Errorf("all pods = %v, want 5", got)
	}
	stats, err := c.Stats(ctx)
	if err != nil || stats.Namespaces != 3 || stats.Pods != 5 || stats.Deployments != 2 || stats.Services != 2 {
		t.Errorf("stats = %+v, %v", stats, err)
	}
}

func TestFakeClientSeedRelations(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	ctx := context.Background()
	containers, err := c.ContainersForPod(ctx, tuitest.NamespaceDemo, tuitest.PodMulti)
	if err != nil || !slices.Equal(containers, []string{tuitest.ContainerApp, tuitest.ContainerSidecar}) {
		t.Errorf("multi containers = %v, %v", containers, err)
	}
	web, err := c.ResolveLogTarget(ctx, k8s.LogKindDeployment, tuitest.NamespaceDemo, tuitest.DeploymentWeb)
	if err != nil || !slices.Equal(web.PodNames(), []string{tuitest.PodWeb1, tuitest.PodWeb2}) || len(web.Skipped) != 0 {
		t.Errorf("web deployment target = %+v, %v", web, err)
	}
	multi, err := c.ResolveLogTarget(ctx, k8s.LogKindDeployment, tuitest.NamespaceDemo, tuitest.DeploymentMulti)
	if err != nil || !slices.Equal(multi.PodNames(), []string{tuitest.PodMulti}) || len(multi.Containers) != 2 {
		t.Errorf("multi deployment target = %+v, %v", multi, err)
	}
	svc, err := c.ResolveLogTarget(ctx, k8s.LogKindService, tuitest.NamespaceDemo, tuitest.ServiceMulti)
	if err != nil || !slices.Equal(svc.PodNames(), []string{tuitest.PodMulti}) || len(svc.Containers) != 2 {
		t.Errorf("multi service target = %+v, %v", svc, err)
	}
}

func TestFakeClientMetrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := tuitest.NewFakeClient(t)
	all, err := c.ListPodMetrics(ctx, "")
	if err != nil || len(all) != 5 {
		t.Fatalf("pod metrics = %v, %v", all, err)
	}
	demo, err := c.ListPodMetrics(ctx, tuitest.NamespaceDemo)
	if err != nil || len(demo) != 4 {
		t.Errorf("demo pod metrics = %v, %v", demo, err)
	}
	nodes, err := c.ListNodeMetrics(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].Name != tuitest.NodeName {
		t.Errorf("node metrics = %v, %v", nodes, err)
	}

	missing := tuitest.NewFakeClient(t, tuitest.WithMetricsStatus(http.StatusNotFound))
	if _, err := missing.ListPodMetrics(ctx, ""); err == nil || !strings.Contains(err.Error(), "could not find") {
		t.Errorf("404 metrics error = %v, want the not-found wording", err)
	}
}

func TestFakeClientOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	empty := tuitest.NewFakeClient(t, tuitest.WithObjects(), tuitest.WithContext("other"))
	if got := podNames(t, empty, ""); len(got) != 0 {
		t.Errorf("WithObjects() pods = %v, want none", got)
	}
	if empty.Context != "other" {
		t.Errorf("WithContext: Context = %q", empty.Context)
	}

	denied := tuitest.NewFakeClient(t, tuitest.WithReactor("list", "pods",
		func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") }))
	if _, err := denied.ListPods(ctx, ""); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("reactor error = %v, want forbidden", err)
	}

	cases := []struct {
		name string
		opts []tuitest.FakeOption
		want string
	}{
		{name: "default body", want: "fake logs"},
		{name: "custom body", opts: []tuitest.FakeOption{tuitest.WithLogs("one\ntwo\n")}, want: "one\ntwo\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t, tc.opts...)
			rc, err := c.StreamPodLogs(ctx, tuitest.NamespaceDemo, tuitest.PodWeb1, 10, "")
			if err != nil {
				t.Fatalf("StreamPodLogs: %v", err)
			}
			defer func() { _ = rc.Close() }()
			body, err := io.ReadAll(rc)
			if err != nil || string(body) != tc.want {
				t.Errorf("log body = %q, %v; want %q", body, err, tc.want)
			}
		})
	}
}

func TestWriteKubeconfig(t *testing.T) {
	t.Parallel()

	path, err := tuitest.WriteKubeconfig(t.TempDir(), "beta", "alpha", "beta")
	if err != nil {
		t.Fatalf("WriteKubeconfig: %v", err)
	}
	ctxs, err := k8s.ListContexts(path)
	if err != nil || len(ctxs) != 2 {
		t.Fatalf("ListContexts = %v, %v", ctxs, err)
	}
	for _, c := range ctxs {
		if c.Current != (c.Name == "beta") {
			t.Errorf("context %q current=%v", c.Name, c.Current)
		}
	}
	client, err := k8s.LoadFromKubeconfigContext(path, "alpha")
	if err != nil || client.Context != "alpha" {
		t.Fatalf("LoadFromKubeconfigContext = %v, %v", client, err)
	}
}
