package tuitest

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Names in the demo seed. The fake clientset lists objects sorted by
// namespace, then name, so the row order in every view is fixed:
// pods in demo are crash-0, multi-0, web-1, web-2.
const (
	NamespaceDefault = "default"
	NamespaceDemo    = "demo"
	NamespaceSystem  = "kube-system"

	PodCrash   = "crash-0"
	PodMulti   = "multi-0"
	PodWeb1    = "web-1"
	PodWeb2    = "web-2"
	PodCoreDNS = "coredns-0"

	DeploymentMulti = "multi"
	DeploymentWeb   = "web"

	ServiceMulti = "multi"
	ServiceWeb   = "web"

	ContainerApp     = "app"
	ContainerSidecar = "sidecar"
	ContainerNginx   = "nginx"
	ContainerCoreDNS = "coredns"

	// ServiceWebPort is the first (and only) port of the web service.
	ServiceWebPort = 80
	// ServiceMultiPort is the first (and only) port of the multi service.
	ServiceMultiPort = 8080
)

// seedAge keeps the AGE column stable at "1h" for the length of a test run.
const seedAge = 90 * time.Minute

// DemoObjects returns the default seed: three namespaces, five pods (one of
// them with two containers, one crash-looping), two deployments and two
// services selecting them. Every call returns fresh objects.
func DemoObjects() []runtime.Object {
	web := map[string]string{"app": "web"}
	multi := map[string]string{"app": "multi"}
	return []runtime.Object{
		Namespace(NamespaceDefault),
		Namespace(NamespaceDemo),
		Namespace(NamespaceSystem),
		Pod(NamespaceDemo, PodWeb1, web, ContainerNginx),
		Pod(NamespaceDemo, PodWeb2, web, ContainerNginx),
		Pod(NamespaceDemo, PodMulti, multi, ContainerApp, ContainerSidecar),
		crashLooping(Pod(NamespaceDemo, PodCrash, map[string]string{"app": "crash"}, ContainerApp)),
		Pod(NamespaceSystem, PodCoreDNS, map[string]string{"k8s-app": "kube-dns"}, ContainerCoreDNS),
		Deployment(NamespaceDemo, DeploymentWeb, 2, web, ContainerNginx),
		Deployment(NamespaceDemo, DeploymentMulti, 1, multi, ContainerApp, ContainerSidecar),
		Service(NamespaceDemo, ServiceWeb, ServiceWebPort, web),
		Service(NamespaceDemo, ServiceMulti, ServiceMultiPort, multi),
	}
}

func created() metav1.Time {
	return metav1.NewTime(time.Now().Add(-seedAge))
}

// Namespace returns an active namespace.
func Namespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: created()},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}
}

// Pod returns a running, ready pod with the given labels and containers.
func Pod(namespace, name string, labels map[string]string, containers ...string) *corev1.Pod {
	specs := make([]corev1.Container, 0, len(containers))
	statuses := make([]corev1.ContainerStatus, 0, len(containers))
	for _, c := range containers {
		specs = append(specs, corev1.Container{Name: c, Image: c + ":latest"})
		statuses = append(statuses, corev1.ContainerStatus{
			Name:  c,
			Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			Labels:            labels,
			CreationTimestamp: created(),
		},
		Spec: corev1.PodSpec{Containers: specs, NodeName: "node-1"},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			PodIP:             "10.42.0.10",
			ContainerStatuses: statuses,
		},
	}
}

func crashLooping(p *corev1.Pod) *corev1.Pod {
	out := p.DeepCopy()
	for i := range out.Status.ContainerStatuses {
		out.Status.ContainerStatuses[i].Ready = false
		out.Status.ContainerStatuses[i].RestartCount = 7
		out.Status.ContainerStatuses[i].State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
		}
	}
	return out
}

// Deployment returns a fully rolled-out deployment selecting labels.
func Deployment(namespace, name string, replicas int32, labels map[string]string, containers ...string) *appsv1.Deployment {
	specs := make([]corev1.Container, 0, len(containers))
	for _, c := range containers {
		specs = append(specs, corev1.Container{Name: c, Image: c + ":latest"})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels, CreationTimestamp: created()},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       corev1.PodSpec{Containers: specs},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          replicas,
			ReadyReplicas:     replicas,
			UpdatedReplicas:   replicas,
			AvailableReplicas: replicas,
		},
	}
}

// Service returns a ClusterIP service exposing one TCP port.
func Service(namespace, name string, port int32, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, CreationTimestamp: created()},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.43.0.10",
			Selector:  selector,
			Ports: []corev1.ServicePort{{
				Port:       port,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt32(port),
			}},
		},
	}
}
