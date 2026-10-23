package k8s

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// ErrYAMLKind is returned for a kind ObjectYAML does not fetch.
var ErrYAMLKind = errors.New("unsupported kind for yaml")

// ObjectYAML returns the object as `kubectl get <kind> <name> -o yaml`
// prints it: apiVersion and kind set, keys in order, and managedFields
// left out as kubectl leaves them out by default. kind is the lower-case
// name the kubectl command uses: pod, deployment, statefulset, daemonset,
// job, cronjob, service, namespace, node or event. namespace is ignored for a namespace and a node, which
// have none.
func (c *Client) ObjectYAML(ctx context.Context, kind, namespace, name string) (string, error) {
	obj, err := c.getObject(ctx, kind, namespace, name)
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	return string(out), nil
}

func (c *Client) getObject(ctx context.Context, kind, namespace, name string) (runtime.Object, error) {
	get := metav1.GetOptions{}
	switch kind {
	case "pod":
		p, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, get)
		return asPrinted(p, err, kind, corev1.SchemeGroupVersion.WithKind("Pod"))
	case "deployment":
		d, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, get)
		return asPrinted(d, err, kind, appsv1.SchemeGroupVersion.WithKind("Deployment"))
	case "statefulset":
		s, err := c.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, get)
		return asPrinted(s, err, kind, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	case "daemonset":
		d, err := c.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, get)
		return asPrinted(d, err, kind, appsv1.SchemeGroupVersion.WithKind("DaemonSet"))
	case "job":
		j, err := c.Clientset.BatchV1().Jobs(namespace).Get(ctx, name, get)
		return asPrinted(j, err, kind, batchv1.SchemeGroupVersion.WithKind("Job"))
	case "cronjob":
		j, err := c.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, get)
		return asPrinted(j, err, kind, batchv1.SchemeGroupVersion.WithKind("CronJob"))
	case "service":
		s, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, get)
		return asPrinted(s, err, kind, corev1.SchemeGroupVersion.WithKind("Service"))
	case "namespace":
		n, err := c.Clientset.CoreV1().Namespaces().Get(ctx, name, get)
		return asPrinted(n, err, kind, corev1.SchemeGroupVersion.WithKind("Namespace"))
	case "node":
		n, err := c.Clientset.CoreV1().Nodes().Get(ctx, name, get)
		return asPrinted(n, err, kind, corev1.SchemeGroupVersion.WithKind("Node"))
	case "event":
		e, err := c.Clientset.CoreV1().Events(namespace).Get(ctx, name, get)
		return asPrinted(e, err, kind, corev1.SchemeGroupVersion.WithKind("Event"))
	}
	return nil, fmt.Errorf("%w: %s", ErrYAMLKind, kind)
}

// printable is a typed API object: its type and metadata can be set.
type printable interface {
	runtime.Object
	metav1.Object
}

// asPrinted readies a fetched object for printing as kubectl prints it.
// A typed client leaves apiVersion and kind empty; the object is the
// client's fresh copy, so setting them changes nothing anyone else holds.
func asPrinted(obj printable, err error, kind string, gvk schema.GroupVersionKind) (runtime.Object, error) {
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", kind, err)
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	obj.SetManagedFields(nil)
	return obj, nil
}
