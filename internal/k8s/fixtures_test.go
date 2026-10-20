package k8s_test

import (
	"net/http"
	"slices"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// granted is the one namespace k8stest.RestrictToNamespace lets through in
// these tests.
const granted = "k4s-demo"

// clusterObjects seeds two namespaces with one pod, deployment and service
// each, so every list has exactly one item per namespace.
func clusterObjects() []runtime.Object {
	return []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: granted}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "other"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "other"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: granted}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "other"}},
	}
}

// pathRecorder remembers the request paths a handler saw.
type pathRecorder struct {
	mu    sync.Mutex
	paths []string
	next  http.Handler
}

func (p *pathRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.paths = append(p.paths, r.URL.Path)
	p.mu.Unlock()
	p.next.ServeHTTP(w, r)
}

func (p *pathRecorder) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.paths)
}
