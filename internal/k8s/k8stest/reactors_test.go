package k8stest_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

const granted = "k4s-demo"

// call is one clientset request; it returns the error the caller would see.
type call func(context.Context, kubernetes.Interface) error

func listPods(ns string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		_, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		return err
	}
}

func getPod(ns, name string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		_, err := cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		return err
	}
}

func listDeployments(ns string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		_, err := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		return err
	}
}

func listServices(ns string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		_, err := cs.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		return err
	}
}

func listNamespaces(ctx context.Context, cs kubernetes.Interface) error {
	_, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	return err
}

func watchPods(ns string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		w, err := cs.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		w.Stop()
		return nil
	}
}

func createPod(ns string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: ns}}
		_, err := cs.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
		return err
	}
}

func updatePod(ns, name string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		_, err := cs.CoreV1().Pods(ns).Update(ctx, pod, metav1.UpdateOptions{})
		return err
	}
}

func patchPod(ns, name string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		_, err := cs.CoreV1().Pods(ns).Patch(ctx, name, types.MergePatchType, []byte(`{}`), metav1.PatchOptions{})
		return err
	}
}

func deletePod(ns, name string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		return cs.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
	}
}

func podLogs(ns, name string) call {
	return func(ctx context.Context, cs kubernetes.Interface) error {
		rc, err := cs.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{}).Stream(ctx)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, rc)
		return rc.Close()
	}
}

func seed() []runtime.Object {
	return []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: granted}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "other"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
	}
}

func TestRestrictToNamespace(t *testing.T) {
	t.Parallel()

	user := k8stest.User
	cases := []struct {
		name    string
		call    call
		wantErr string // empty: the request is allowed
	}{
		{name: "list pods in the granted namespace", call: listPods(granted)},
		{name: "get pod in the granted namespace", call: getPod(granted, "web")},
		{name: "create pod in the granted namespace", call: createPod(granted)},
		{name: "watch pods in the granted namespace", call: watchPods(granted)},
		{name: "pod logs in the granted namespace", call: podLogs(granted, "web")},
		{name: "list deployments in the granted namespace", call: listDeployments(granted)},
		{
			name:    "list pods across all namespaces",
			call:    listPods(""),
			wantErr: `pods is forbidden: User "` + user + `" cannot list resource "pods" in API group "" at the cluster scope`,
		},
		{
			name:    "list pods in another namespace",
			call:    listPods("other"),
			wantErr: `pods is forbidden: User "` + user + `" cannot list resource "pods" in API group "" in the namespace "other"`,
		},
		{
			name:    "get pod in another namespace",
			call:    getPod("other", "db"),
			wantErr: `pods "db" is forbidden: User "` + user + `" cannot get resource "pods" in API group "" in the namespace "other"`,
		},
		{
			name:    "update pod in another namespace",
			call:    updatePod("other", "db"),
			wantErr: `pods "db" is forbidden: User "` + user + `" cannot update resource "pods" in API group "" in the namespace "other"`,
		},
		{
			name:    "create pod in another namespace",
			call:    createPod("other"),
			wantErr: `pods is forbidden: User "` + user + `" cannot create resource "pods" in API group "" in the namespace "other"`,
		},
		{
			name:    "list deployments across all namespaces",
			call:    listDeployments(""),
			wantErr: `deployments.apps is forbidden: User "` + user + `" cannot list resource "deployments" in API group "apps" at the cluster scope`,
		},
		{
			name:    "list services across all namespaces",
			call:    listServices(""),
			wantErr: `services is forbidden: User "` + user + `" cannot list resource "services" in API group "" at the cluster scope`,
		},
		{
			name:    "list cluster-scoped namespaces",
			call:    listNamespaces,
			wantErr: `namespaces is forbidden: User "` + user + `" cannot list resource "namespaces" in API group "" at the cluster scope`,
		},
		{
			name:    "watch pods across all namespaces",
			call:    watchPods(""),
			wantErr: `pods is forbidden: User "` + user + `" cannot watch resource "pods" in API group "" at the cluster scope`,
		},
		{
			name:    "pod logs in another namespace",
			call:    podLogs("other", "db"),
			wantErr: `cannot get resource "pods/log" in API group "" in the namespace "other"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := fake.NewSimpleClientset(seed()...)
			k8stest.RestrictToNamespace(cs, granted)

			err := tc.call(context.Background(), cs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("allowed request failed: %v", err)
				}
				return
			}
			if !apierrors.IsForbidden(err) {
				t.Fatalf("err = %v, want Forbidden", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestRestrictToNamespaceStillServesTheTracker(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(seed()...)
	k8stest.RestrictToNamespace(cs, granted)

	list, err := cs.CoreV1().Pods(granted).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "web" {
		t.Fatalf("items = %v, want only web", list.Items)
	}
}

func TestForbid(t *testing.T) {
	t.Parallel()

	user := k8stest.User
	cases := []struct {
		name     string
		verb     string
		resource string
		call     call
		wantErr  string
	}{
		{
			name: "denied verb and resource", verb: "list", resource: "services", call: listServices(granted),
			wantErr: `services is forbidden: User "` + user + `" cannot list resource "services" in API group "" in the namespace "k4s-demo"`,
		},
		{name: "other resource passes", verb: "list", resource: "services", call: listPods(granted)},
		{name: "other verb passes", verb: "list", resource: "services", call: getPod(granted, "web")},
		{
			name: "wildcard verb covers watch", verb: "*", resource: "pods", call: watchPods(""),
			wantErr: `cannot watch resource "pods" in API group "" at the cluster scope`,
		},
		{
			name: "watch verb covers watch", verb: "watch", resource: "pods", call: watchPods(granted),
			wantErr: `cannot watch resource "pods"`,
		},
		{name: "list verb leaves watch alone", verb: "list", resource: "pods", call: watchPods(granted)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := fake.NewSimpleClientset(seed()...)
			k8stest.Forbid(cs, tc.verb, tc.resource)

			err := tc.call(context.Background(), cs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("request outside the denial failed: %v", err)
				}
				return
			}
			if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want Forbidden containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestUnauthorized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call call
	}{
		{name: "list", call: listPods(granted)},
		{name: "list across all namespaces", call: listPods("")},
		{name: "get", call: getPod(granted, "web")},
		{name: "create", call: createPod(granted)},
		{name: "watch", call: watchPods(granted)},
		{name: "cluster-scoped list", call: listNamespaces},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := fake.NewSimpleClientset(seed()...)
			k8stest.Unauthorized(cs)

			err := tc.call(context.Background(), cs)
			if !apierrors.IsUnauthorized(err) {
				t.Fatalf("err = %v, want Unauthorized", err)
			}
			if err.Error() != "Unauthorized" {
				t.Fatalf("message = %q, want the apiserver's %q", err, "Unauthorized")
			}
		})
	}
}

func TestSlowerThanDeadline(t *testing.T) {
	t.Parallel()

	const after = 30 * time.Millisecond
	cases := []struct {
		name    string
		call    call
		wantOp  string
		wantURL string
	}{
		{name: "namespaced list", call: listPods(granted), wantOp: "Get", wantURL: "/api/v1/namespaces/k4s-demo/pods"},
		{name: "list across all namespaces", call: listPods(""), wantOp: "Get", wantURL: "/api/v1/pods"},
		{name: "named get", call: getPod(granted, "web"), wantOp: "Get", wantURL: "/api/v1/namespaces/k4s-demo/pods/web"},
		{name: "grouped resource", call: listDeployments(""), wantOp: "Get", wantURL: "/apis/apps/v1/deployments"},
		{name: "create", call: createPod(granted), wantOp: "Post", wantURL: "/api/v1/namespaces/k4s-demo/pods"},
		{name: "update", call: updatePod(granted, "web"), wantOp: "Put", wantURL: "/api/v1/namespaces/k4s-demo/pods/web"},
		{name: "patch", call: patchPod(granted, "web"), wantOp: "Patch", wantURL: "/api/v1/namespaces/k4s-demo/pods/web"},
		{name: "delete", call: deletePod(granted, "web"), wantOp: "Delete", wantURL: "/api/v1/namespaces/k4s-demo/pods/web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := fake.NewSimpleClientset(seed()...)
			k8stest.SlowerThanDeadline(cs, after)

			start := time.Now()
			err := tc.call(context.Background(), cs)
			if elapsed := time.Since(start); elapsed < after {
				t.Fatalf("answered after %v, want at least %v", elapsed, after)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want context.DeadlineExceeded", err)
			}
			var uerr *url.Error
			if !errors.As(err, &uerr) {
				t.Fatalf("err = %T, want *url.Error like client-go returns", err)
			}
			if uerr.Op != tc.wantOp || !strings.HasSuffix(uerr.URL, tc.wantURL) {
				t.Fatalf("url.Error = %s %s, want %s ...%s", uerr.Op, uerr.URL, tc.wantOp, tc.wantURL)
			}
			var nerr net.Error
			if !errors.As(err, &nerr) || !nerr.Timeout() {
				t.Fatalf("err = %v, want a net.Error that reports Timeout", err)
			}
		})
	}
}
