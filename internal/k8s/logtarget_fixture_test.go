package k8s_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ktesting "k8s.io/client-go/testing"
)

// The growlytics fixture is the owner's cluster from the live session: the
// app deployment runs 2 pods in its current ReplicaSet next to an evicted
// one, the old ReplicaSet is scaled to 0, and 13 finished pods from 6
// CronJobs carry the same app label.
const (
	glNS      = "growlytics"
	glApp     = "growlytics-app"
	glHash    = "7d9f8c6b5d"
	glOldHash = "5c8d7b9f4"
	hashKey   = "pod-template-hash"
)

var (
	glRunning = []string{glApp + "-" + glHash + "-x2x9k", glApp + "-" + glHash + "-q8r7m"}
	glEvicted = glApp + "-" + glHash + "-zk4lp"
)

func growlyticsObjects() []runtime.Object {
	const jobPods = 13
	dep := deployment(glNS, glApp, "uid-deploy", map[string]string{"app": glApp})
	objs := make([]runtime.Object, 0, 4+len(glRunning)+1+jobPods)
	objs = append(objs,
		dep,
		replicaSet(dep, glHash, "uid-rs-new", "3", 2),
		replicaSet(dep, glOldHash, "uid-rs-old", "2", 0),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: glApp, Namespace: glNS},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": glApp}},
		},
	)
	for _, name := range glRunning {
		objs = append(objs, rsPod(glNS, name, glApp, glHash, "uid-rs-new", corev1.PodRunning))
	}
	evicted := rsPod(glNS, glEvicted, glApp, glHash, "uid-rs-new", corev1.PodFailed)
	evicted.Status.Reason = "Evicted"
	objs = append(objs, evicted)
	for i := range jobPods {
		job := fmt.Sprintf("growlytics-cron%d-2934567%02d", i%6, i)
		objs = append(objs, ownedPod(glNS, fmt.Sprintf("%s-%05d", job, i), map[string]string{"app": glApp, "job-name": job},
			controllerRef("batch/v1", "Job", job, types.UID("uid-job-"+job)), corev1.PodSucceeded))
	}
	return objs
}

func deployment(ns, name string, uid types.UID, matchLabels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: matchLabels},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
		},
	}
}

func replicaSet(dep *appsv1.Deployment, hash string, uid types.UID, revision string, replicas int32) *appsv1.ReplicaSet {
	labels := map[string]string{hashKey: hash}
	for k, v := range dep.Spec.Selector.MatchLabels {
		labels[k] = v
	}
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            dep.Name + "-" + hash,
			Namespace:       dep.Namespace,
			UID:             uid,
			Labels:          labels,
			Annotations:     map[string]string{"deployment.kubernetes.io/revision": revision},
			OwnerReferences: []metav1.OwnerReference{controllerRef("apps/v1", "Deployment", dep.Name, dep.UID)},
		},
		Status: appsv1.ReplicaSetStatus{Replicas: replicas},
	}
}

func rsPod(ns, name, app, hash string, rsUID types.UID, phase corev1.PodPhase) *corev1.Pod {
	ref := controllerRef("apps/v1", "ReplicaSet", app+"-"+hash, rsUID)
	return ownedPod(ns, name, map[string]string{"app": app, hashKey: hash}, ref, phase)
}

func ownedPod(ns, name string, labels map[string]string, ref metav1.OwnerReference, phase corev1.PodPhase) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels, CreationTimestamp: metav1.NewTime(time.Unix(1_700_000_000, 0))},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		Status:     corev1.PodStatus{Phase: phase},
	}
	if ref.Name != "" {
		p.OwnerReferences = []metav1.OwnerReference{ref}
	}
	return p
}

func controllerRef(apiVersion, kind, name string, uid types.UID) metav1.OwnerReference {
	yes := true
	return metav1.OwnerReference{APIVersion: apiVersion, Kind: kind, Name: name, UID: uid, Controller: &yes}
}

// apiCall is the part of a recorded fake action the call-budget checks read.
type apiCall struct {
	Verb, Resource, Selector, ResourceVersion string
}

func checkCalls(t *testing.T, actions []ktesting.Action, want []apiCall) {
	t.Helper()
	if got := apiCalls(actions); !slices.Equal(got, want) {
		t.Errorf("API calls = %+v\nwant          %+v", got, want)
	}
}

func apiCalls(actions []ktesting.Action) []apiCall {
	out := make([]apiCall, 0, len(actions))
	for _, a := range actions {
		c := apiCall{Verb: a.GetVerb(), Resource: a.GetResource().Resource}
		if l, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
			c.Selector = l.GetListOptions().LabelSelector
			c.ResourceVersion = l.GetListOptions().ResourceVersion
		}
		out = append(out, c)
	}
	return out
}
