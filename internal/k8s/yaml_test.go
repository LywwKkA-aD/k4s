package k8s_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func yamlObjects() []runtime.Object {
	managed := []metav1.ManagedFieldsEntry{{Manager: "kubectl-client-side-apply", Operation: metav1.ManagedFieldsOperationUpdate}}
	meta := func(ns, name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID("u-" + name), ManagedFields: managed, Labels: map[string]string{"app": name}}
	}
	return []runtime.Object{
		&corev1.Namespace{ObjectMeta: meta("", granted)},
		&corev1.Pod{
			ObjectMeta: meta(granted, "web-1"),
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.27"}}},
		},
		&appsv1.Deployment{ObjectMeta: meta(granted, "web")},
		&corev1.Service{ObjectMeta: meta(granted, "web"), Spec: corev1.ServiceSpec{ClusterIP: "10.43.0.10"}},
	}
}

func TestObjectYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind, namespace, name string
		head                  string
		want                  []string
	}{
		{kind: "pod", namespace: granted, name: "web-1", head: "apiVersion: v1\nkind: Pod\nmetadata:\n", want: []string{"  name: web-1\n", "  - image: nginx:1.27\n"}},
		{kind: "deployment", namespace: granted, name: "web", head: "apiVersion: apps/v1\nkind: Deployment\n"},
		{kind: "service", namespace: granted, name: "web", head: "apiVersion: v1\nkind: Service\n", want: []string{"  clusterIP: 10.43.0.10\n"}},
		{kind: "namespace", name: granted, head: "apiVersion: v1\nkind: Namespace\n", want: []string{"  name: " + granted + "\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(yamlObjects()...)
			out, err := c.ObjectYAML(context.Background(), tc.kind, tc.namespace, tc.name)
			if err != nil {
				t.Fatalf("ObjectYAML: %v", err)
			}
			if !strings.HasPrefix(out, tc.head) {
				t.Errorf("YAML does not start with %q:\n%s", tc.head, out)
			}
			for _, w := range append(tc.want, "  uid: u-"+tc.name+"\n", "    app: "+tc.name+"\n") {
				if !strings.Contains(out, w) {
					t.Errorf("YAML lacks %q:\n%s", w, out)
				}
			}
			if strings.Contains(out, "managedFields") || strings.Contains(out, "kubectl-client-side-apply") {
				t.Errorf("managedFields are shown; kubectl hides them by default:\n%s", out)
			}
			var back map[string]any
			if err := yaml.Unmarshal([]byte(out), &back); err != nil {
				t.Errorf("YAML does not parse back: %v", err)
			}
		})
	}
}

func TestObjectYAMLErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, kind, objName string
		is                  func(error) bool
		prefix              string
	}{
		{name: "missing pod", kind: "pod", objName: "gone", is: apierrors.IsNotFound, prefix: "get pod: "},
		{name: "missing deployment", kind: "deployment", objName: "gone", is: apierrors.IsNotFound, prefix: "get deployment: "},
		{name: "missing service", kind: "service", objName: "gone", is: apierrors.IsNotFound, prefix: "get service: "},
		{name: "missing namespace", kind: "namespace", objName: "gone", is: apierrors.IsNotFound, prefix: "get namespace: "},
		{
			name: "unsupported kind", kind: "secret", objName: "token",
			is:     func(err error) bool { return errors.Is(err, k8s.ErrYAMLKind) },
			prefix: "unsupported kind for yaml: secret",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(yamlObjects()...)
			out, err := c.ObjectYAML(context.Background(), tc.kind, granted, tc.objName)
			if !tc.is(err) || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("err = %v, want %q classified as %s", err, tc.prefix, tc.name)
			}
			if out != "" {
				t.Errorf("output next to an error:\n%s", out)
			}
		})
	}
}

func TestObjectYAMLForbidden(t *testing.T) {
	t.Parallel()

	c, cs := k8stest.NewFakeClient(yamlObjects()...)
	k8stest.RestrictToNamespace(cs, "other")
	_, err := c.ObjectYAML(context.Background(), "pod", granted, "web-1")
	if !apierrors.IsForbidden(err) {
		t.Fatalf("err = %v, want Forbidden", err)
	}
}
