package dash

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// A synthetic cluster for tests: each kind is described once and rendered
// both as typed objects (for the fake clientset) and as server-side Table
// rows (for the httptest apiserver), so lookups and Tables agree.

type tCont struct {
	name     string
	init     bool
	waiting  string // waiting reason
	exit     int32  // terminated exit code, with terminated true
	term     bool
	restarts int32
	ready    bool
}

type tPod struct {
	ns, name, node string
	status         string // the STATUS word the Table shows
	ready, of      int
	restarts       int32
	lastRestart    time.Duration // ago; 0 means never
	age            time.Duration
	conts          []tCont
}

type tWorkload struct {
	kind           string // deployment, statefulset, daemonset
	ns, name       string
	ready, desired int32
}

type tCron struct {
	ns, name string
	suspend  bool
	last     time.Duration
}

type tJob struct {
	ns, name  string
	active    int32
	failedAgo time.Duration // 0: not failed
}

type tNode struct {
	name        string
	roles       []string
	ready       corev1.ConditionStatus
	pressures   []corev1.NodeConditionType
	cpu, mem    string // allocatable
	pods        int64
	age         time.Duration
	annotations map[string]string
	unsched     bool
	useCPU      string // metrics; "" reports no metrics
	useMem      string
}

type tEvent struct {
	ns, kind, name, fieldPath string
	reason, message           string
	count                     int32
	ago                       time.Duration
}

type tSvc struct {
	ns, name string
	typ      corev1.ServiceType
	pending  bool
}

type tPVC struct{ ns, name, status string }

type tNS struct {
	name, status string
	age          time.Duration
}

type tHelm struct{ name, job string }

type tSnap struct {
	node string
	ago  time.Duration
}

type tPodUse struct {
	ns, name string
	cpu, mem string
}

type cluster struct {
	now        time.Time
	git        string
	user       string
	metrics    bool
	readyz     string
	pods       []tPod
	workloads  []tWorkload
	crons      []tCron
	jobs       []tJob
	nodes      []tNode
	events     []tEvent
	svcs       []tSvc
	pvcs       []tPVC
	namespaces []tNS
	helm       []tHelm // nil: no helm.cattle.io API
	snaps      []tSnap // nil: no k3s.cattle.io etcdsnapshotfiles API
	podUse     []tPodUse
}

func objMeta(ns, name string, created time.Time) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: ns, Name: name, CreationTimestamp: metav1.NewTime(created),
		UID: types.UID(fmt.Sprintf("uid-%s-%s", ns, name)),
	}
}

// objects renders the typed half of the cluster.
func (c cluster) objects() []runtime.Object {
	out := make([]runtime.Object, 0, len(c.pods)+len(c.workloads)+len(c.crons)+len(c.jobs)+len(c.nodes)+len(c.events)+len(c.svcs))
	for _, p := range c.pods {
		out = append(out, c.typedPod(p))
	}
	for _, w := range c.workloads {
		out = append(out, c.typedWorkload(w))
	}
	for _, cj := range c.crons {
		out = append(out, &batchv1.CronJob{ObjectMeta: objMeta(cj.ns, cj.name, c.now.Add(-30*day)), Spec: batchv1.CronJobSpec{Suspend: &cj.suspend}})
	}
	for _, j := range c.jobs {
		out = append(out, c.typedJob(j))
	}
	for _, n := range c.nodes {
		out = append(out, c.typedNode(n))
	}
	for i, e := range c.events {
		out = append(out, c.typedEvent(i, e))
	}
	for _, s := range c.svcs {
		svc := &corev1.Service{ObjectMeta: objMeta(s.ns, s.name, c.now.Add(-day)), Spec: corev1.ServiceSpec{Type: s.typ}}
		if s.typ == corev1.ServiceTypeLoadBalancer && !s.pending {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "10.0.0.50"}}
		}
		out = append(out, svc)
	}
	return out
}

func (c cluster) typedPod(p tPod) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: objMeta(p.ns, p.name, c.now.Add(-p.age)), Spec: corev1.PodSpec{NodeName: p.node}}
	pod.Status.Phase = corev1.PodRunning
	switch p.status {
	case "Completed":
		pod.Status.Phase = corev1.PodSucceeded
	case "Pending", "ContainerCreating":
		pod.Status.Phase = corev1.PodPending
	case "Evicted":
		pod.Status.Phase, pod.Status.Reason = corev1.PodFailed, "Evicted"
	}
	conts := p.conts
	if len(conts) == 0 {
		conts = []tCont{{name: "app", ready: p.ready > 0, restarts: p.restarts}}
	}
	for _, ct := range conts {
		st := corev1.ContainerStatus{Name: ct.name, Ready: ct.ready, RestartCount: ct.restarts}
		switch {
		case ct.waiting != "":
			st.State.Waiting = &corev1.ContainerStateWaiting{Reason: ct.waiting}
		case ct.term:
			st.State.Terminated = &corev1.ContainerStateTerminated{ExitCode: ct.exit}
		default:
			st.State.Running = &corev1.ContainerStateRunning{}
		}
		if ct.init {
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{Name: ct.name})
			pod.Status.InitContainerStatuses = append(pod.Status.InitContainerStatuses, st)
			continue
		}
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: ct.name})
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, st)
	}
	return pod
}

func (c cluster) typedWorkload(w tWorkload) runtime.Object {
	m := objMeta(w.ns, w.name, c.now.Add(-10*day))
	n := w.desired
	switch w.kind {
	case "statefulset":
		return &appsv1.StatefulSet{ObjectMeta: m, Spec: appsv1.StatefulSetSpec{Replicas: &n}, Status: appsv1.StatefulSetStatus{ReadyReplicas: w.ready}}
	case "daemonset":
		return &appsv1.DaemonSet{ObjectMeta: m, Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: n, NumberReady: w.ready}}
	default:
		return &appsv1.Deployment{ObjectMeta: m, Spec: appsv1.DeploymentSpec{Replicas: &n}, Status: appsv1.DeploymentStatus{ReadyReplicas: w.ready}}
	}
}

func (c cluster) typedJob(j tJob) *batchv1.Job {
	job := &batchv1.Job{ObjectMeta: objMeta(j.ns, j.name, c.now.Add(-2*time.Hour)), Status: batchv1.JobStatus{Active: j.active}}
	if j.failedAgo > 0 {
		job.Status.Failed = 1
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(c.now.Add(-j.failedAgo)),
		}}
	}
	return job
}

func (c cluster) typedNode(n tNode) *corev1.Node {
	labels := map[string]string{}
	for _, r := range n.roles {
		labels[roleLabelPrefix+r] = "true"
	}
	node := &corev1.Node{ObjectMeta: objMeta("", n.name, c.now.Add(-n.age)), Spec: corev1.NodeSpec{Unschedulable: n.unsched}}
	node.Labels, node.Annotations = labels, n.annotations
	node.Status.Allocatable = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(n.cpu),
		corev1.ResourceMemory: resource.MustParse(n.mem),
		corev1.ResourcePods:   *resource.NewQuantity(n.pods, resource.DecimalSI),
	}
	node.Status.NodeInfo.KubeletVersion = c.git
	ready := n.ready
	if ready == "" {
		ready = corev1.ConditionTrue
	}
	node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}
	for _, pr := range n.pressures {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{Type: pr, Status: corev1.ConditionTrue})
	}
	return node
}

func (c cluster) typedEvent(i int, e tEvent) *corev1.Event {
	count := max(e.count, 1)
	ev := &corev1.Event{
		ObjectMeta: objMeta(e.ns, fmt.Sprintf("%s.%x", e.name, i), c.now.Add(-e.ago)),
		InvolvedObject: corev1.ObjectReference{
			Kind: e.kind, Namespace: e.ns, Name: e.name, FieldPath: e.fieldPath,
		},
		Type: corev1.EventTypeWarning, Reason: e.reason, Message: e.message, Count: count,
		LastTimestamp: metav1.NewTime(c.now.Add(-e.ago)),
	}
	if e.kind == "Node" {
		ev.Namespace, ev.InvolvedObject.Namespace = "default", ""
	}
	return ev
}
