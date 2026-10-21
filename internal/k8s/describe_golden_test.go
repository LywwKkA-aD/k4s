package k8s

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// updateGolden rewrites testdata/describe_*.golden:
// go test ./internal/k8s -run Golden -update
var updateGolden = flag.Bool("update", false, "rewrite testdata/*.golden with the current output")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (rerun with -update to create it): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s differs (rerun with -update to accept)\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

func fakeClient(objs ...runtime.Object) *Client {
	return &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}
}

// TestDescribeGoldens pins the built-in describer, the fallback when
// kubectl is missing or fails, in kubectl's own layout.
func TestDescribeGoldens(t *testing.T) {
	t.Parallel()

	crash, initFail, sidecar := crashLoopPod(), initFailurePod(), sidecarPod()
	dep, svc := webDeployment(), lbService()
	cases := []struct {
		name     string
		objs     []runtime.Object
		describe func(*Client) (string, error)
	}{
		{
			name: "describe_pod_crashloop",
			objs: append([]runtime.Object{crash}, crashLoopEvents(crash)...),
			describe: func(c *Client) (string, error) {
				return c.describePod(context.Background(), crash.Namespace, crash.Name, describeNow)
			},
		},
		{
			name: "describe_pod_init_failure",
			objs: append([]runtime.Object{initFail}, initFailureEvents(initFail)...),
			describe: func(c *Client) (string, error) {
				return c.describePod(context.Background(), initFail.Namespace, initFail.Name, describeNow)
			},
		},
		{
			name: "describe_pod_sidecar",
			objs: []runtime.Object{sidecar},
			describe: func(c *Client) (string, error) {
				return c.describePod(context.Background(), sidecar.Namespace, sidecar.Name, describeNow)
			},
		},
		{
			name: "describe_deployment",
			objs: append([]runtime.Object{dep}, webDeploymentEvents(dep)...),
			describe: func(c *Client) (string, error) {
				return c.describeDeployment(context.Background(), dep.Namespace, dep.Name, describeNow)
			},
		},
		{
			name: "describe_service",
			objs: []runtime.Object{svc},
			describe: func(c *Client) (string, error) {
				return c.describeService(context.Background(), svc.Namespace, svc.Name, describeNow)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := tc.describe(fakeClient(tc.objs...))
			if err != nil {
				t.Fatalf("describe: %v", err)
			}
			golden(t, tc.name, out)
		})
	}
}

// TestDescribeEventsByUID: an event about an earlier pod of the same name
// is not this pod's, and a series counts its repeats.
func TestDescribeEventsByUID(t *testing.T) {
	t.Parallel()

	crash := crashLoopPod()
	out, err := fakeClient(append([]runtime.Object{crash}, crashLoopEvents(crash)...)...).
		describePod(context.Background(), crash.Namespace, crash.Name, describeNow)
	if err != nil {
		t.Fatalf("describePod: %v", err)
	}
	if strings.Contains(out, "an earlier pod's event") {
		t.Errorf("an event of another pod with the same name is shown:\n%s", out)
	}
	if !strings.Contains(out, "51s (x294 over 15h)") {
		t.Errorf("the repeat count of BackOff is missing:\n%s", out)
	}
}

// TestDescribeMirrorPodEvents: a static pod's events name the uid of its
// manifest, which the mirror pod carries in an annotation.
func TestDescribeMirrorPodEvents(t *testing.T) {
	t.Parallel()

	pod := crashLoopPod()
	pod.Annotations = map[string]string{"kubernetes.io/config.mirror": "static-uid"}
	ev := podEvent(pod, "static", "Normal", "Started", "Started container", 1, before(0), before(0))
	ev.InvolvedObject.UID = "static-uid"
	out, err := fakeClient(pod, ev).describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
	if err != nil {
		t.Fatalf("describePod: %v", err)
	}
	if !strings.Contains(out, "Started container") {
		t.Errorf("the mirror pod lost its events:\n%s", out)
	}
}
