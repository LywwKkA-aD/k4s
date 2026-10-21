package dash

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	minute  = time.Minute
	hour    = time.Hour
	k3sGit  = "v1.36.3+k3s1"
	readyOK = "[+]ping ok\n[+]log ok\n[+]etcd ok\n[+]etcd-readiness ok\n[+]informer-sync ok\n" +
		"[+]poststarthook/start-apiserver-admission-initializer ok\n[+]poststarthook/generic-apiserver-start-informers ok\n" +
		"[+]shutdown ok\nreadyz check passed\n"
	// nodeArgs is what k3s writes into k3s.io/node-args; a token must never
	// reach a snapshot, whatever k3s itself masks.
	nodeArgs = `["server","--cluster-init","--token","SECRET-TOKEN-X"]`
)

func run(ns, name, node string, age time.Duration) tPod {
	return tPod{ns: ns, name: name, node: node, status: "Running", ready: 1, of: 1, age: age}
}

func k3sNodes(agentMem string) []tNode {
	return []tNode{
		{name: "k3s-cp-1", roles: []string{"control-plane", "etcd", "master"}, cpu: "4", mem: "16Gi", pods: 250, age: 212 * day,
			annotations: map[string]string{reconcileAnnotation: "2026-10-21T04:00:00Z", "k3s.io/node-args": nodeArgs},
			useCPU:      "1520m", useMem: "11633Mi"},
		{name: "k3s-agent-1", roles: nil, cpu: "4", mem: "8Gi", pods: 110, age: 212 * day, useCPU: "2080m", useMem: agentMem},
		{name: "k3s-agent-2", roles: nil, cpu: "4", mem: "8Gi", pods: 110, age: 98 * day, useCPU: "560m", useMem: "3195Mi"},
	}
}

func k3sSystem() ([]tWorkload, []tPod) {
	w := []tWorkload{
		{"deployment", "kube-system", "coredns", 1, 1},
		{"deployment", "kube-system", "traefik", 1, 1},
		{"deployment", "kube-system", "metrics-server", 1, 1},
		{"deployment", "kube-system", "local-path-provisioner", 1, 1},
		{"daemonset", "kube-system", "svclb-traefik-4a1b2c3d", 3, 3},
	}
	p := []tPod{
		run("kube-system", "coredns-ccb96694c-8xq2z", "k3s-cp-1", 212*day),
		run("kube-system", "metrics-server-5985cbc9d7-9w4kq", "k3s-cp-1", 212*day),
		run("kube-system", "local-path-provisioner-5cf85fd84d-t2xbn", "k3s-cp-1", 212*day),
		run("kube-system", "svclb-traefik-4a1b2c3d-abcde", "k3s-cp-1", 212*day),
		run("kube-system", "svclb-traefik-4a1b2c3d-fghij", "k3s-agent-1", 212*day),
		run("kube-system", "svclb-traefik-4a1b2c3d-klmno", "k3s-agent-2", 98*day),
	}
	return w, p
}

func k3sNamespaces() []tNS {
	return []tNS{
		{"kube-system", "Active", 212 * day}, {"default", "Active", 212 * day}, {"shop", "Active", 90 * day},
		{"monitoring", "Active", 90 * day}, {"staging", "Active", 30 * day}, {"ci", "Active", 30 * day},
		{"growlytics", "Active", 60 * day}, {"old-team", "Terminating", 2 * day},
	}
}

// busyK3s is the spec's busy fixture with the critique's corrections:
// traefik is a Deployment, ServiceLB is a svclb-* DaemonSet, a pod runs two
// containers, an init container crashes and two pods share a name.
func busyK3s(now time.Time) cluster {
	sys, sysPods := k3sSystem()
	traefik := run("kube-system", "traefik-6f5f87584-x7k2p", "k3s-cp-1", 12*day)
	traefik.restarts, traefik.lastRestart = 7, 20*hour
	ingest := run("growlytics", "ingest-6d8c7b5f9-m2kzp", "k3s-agent-2", 2*day)
	ingest.restarts, ingest.lastRestart = 3, 5*hour
	pods := append(sysPods, traefik, ingest,
		tPod{ns: "shop", name: "payments-worker-6c9f7d8b5-xk2lp", node: "k3s-agent-1", status: "CrashLoopBackOff", of: 1,
			restarts: 41, lastRestart: 2 * minute, age: 3 * hour, conts: []tCont{{name: "worker", waiting: "CrashLoopBackOff", restarts: 41}}},
		tPod{ns: "shop", name: "payments-worker-6c9f7d8b5-r8vqn", node: "k3s-agent-2", status: "CrashLoopBackOff", of: 1,
			restarts: 39, lastRestart: 3 * minute, age: 3 * hour, conts: []tCont{{name: "worker", waiting: "CrashLoopBackOff", restarts: 39}}},
		tPod{ns: "monitoring", name: "loki-backend-0", node: "k3s-cp-1", status: "CrashLoopBackOff", ready: 1, of: 2,
			restarts: 12, lastRestart: 30 * time.Second, age: 5 * day,
			conts: []tCont{{name: "loki", ready: true}, {name: "ingester", waiting: "CrashLoopBackOff", restarts: 12}}},
		tPod{ns: "ci", name: "migrate-7d9f8b6c5-q2w3e", node: "k3s-agent-2", status: "Init:CrashLoopBackOff", of: 1,
			restarts: 5, lastRestart: 4 * minute, age: 40 * minute,
			conts: []tCont{{name: "migrate-db", init: true, waiting: "CrashLoopBackOff", restarts: 5}, {name: "app", waiting: "PodInitializing"}}},
		tPod{ns: "staging", name: "postgres-0", node: "k3s-agent-2", status: "CrashLoopBackOff", of: 1,
			restarts: 4, lastRestart: 5 * minute, age: day, conts: []tCont{{name: "postgres", waiting: "CrashLoopBackOff", restarts: 4}}},
		run("shop", "postgres-0", "k3s-agent-1", 30*day),
		tPod{ns: "staging", name: "api-7f6d9c4b8-2mzqt", node: "k3s-agent-1", status: "ImagePullBackOff", of: 1, age: 22 * minute,
			conts: []tCont{{name: "api", waiting: "ImagePullBackOff"}}},
		tPod{ns: "monitoring", name: "grafana-5d8f7c6b9-wq2lx", node: "k3s-cp-1", status: "Running", of: 1, age: 9 * minute},
		tPod{ns: "ci", name: "runner-build-8812-k7x2p", status: "Pending", of: 1, age: 6 * minute},
		tPod{ns: "growlytics", name: "report-28741230-9q8wd", node: "k3s-agent-1", status: "Evicted", of: 1, age: 48 * minute},
		tPod{ns: "growlytics", name: "report-28741290-abcde", node: "k3s-agent-1", status: "Completed", of: 1, age: 3 * minute},
		run("shop", "payments-api-5f7b8c9d4-tq8vn", "k3s-agent-1", 3*day),
		run("growlytics", "web-7c9d8f6b5-p2r7x", "k3s-agent-2", 3*day),
		run("monitoring", "prometheus-k8s-0", "k3s-cp-1", 30*day),
	)
	workloads := append(sys,
		tWorkload{"deployment", "shop", "payments-worker", 0, 2},
		tWorkload{"deployment", "shop", "payments-api", 1, 1},
		tWorkload{"deployment", "staging", "api", 0, 1},
		tWorkload{"deployment", "monitoring", "grafana", 0, 1},
		tWorkload{"deployment", "growlytics", "web", 1, 1},
		tWorkload{"deployment", "growlytics", "ingest", 1, 1},
		tWorkload{"deployment", "ci", "migrate", 0, 1},
		tWorkload{"statefulset", "monitoring", "loki-backend", 0, 1},
		tWorkload{"statefulset", "monitoring", "prometheus-k8s", 1, 1},
		tWorkload{"statefulset", "shop", "postgres", 1, 1},
		tWorkload{"statefulset", "staging", "postgres", 0, 1},
	)
	return cluster{
		now: now, git: k3sGit, user: "devops-friend", metrics: true, readyz: readyOK,
		nodes: k3sNodes("7128Mi"), namespaces: k3sNamespaces(), pods: pods, workloads: workloads,
		crons: []tCron{{"growlytics", "report-weekly", true, 6 * day}, {"growlytics", "report", false, 3 * minute}},
		jobs:  []tJob{{"ci", "nightly-e2e-28741000", 0, 41 * minute}, {"ci", "runner-batch", 1, 0}},
		events: []tEvent{
			{"monitoring", "Pod", "loki-backend-0", "spec.containers{ingester}", "BackOff", "Back-off restarting failed container ingester in pod loki-backend-0", 112, 30 * time.Second},
			{"shop", "Pod", "payments-worker-6c9f7d8b5-xk2lp", "spec.containers{worker}", "BackOff", "Back-off restarting failed container worker", 41, 2 * minute},
			{"ci", "Pod", "runner-build-8812-k7x2p", "", "FailedScheduling", "0/3 nodes are available: 3 Insufficient memory.", 4, 6 * minute},
			{"monitoring", "Pod", "grafana-5d8f7c6b9-wq2lx", "spec.containers{grafana}", "Unhealthy", "Readiness probe failed: HTTP probe failed with statuscode: 503", 27, minute},
			{"staging", "Pod", "api-7f6d9c4b8-2mzqt", "spec.containers{api}", "Failed", `Failed to pull image "registry.example.internal/api:2.4"`, 9, 22 * minute},
			{"ci", "Job", "nightly-e2e-28741000", "", "BackoffLimitExceeded", "Job has reached the specified backoff limit", 1, 41 * minute},
			{"growlytics", "Pod", "report-28741230-9q8wd", "", "Evicted", "The node was low on resource: memory.", 1, 48 * minute},
			{"growlytics", "Pod", "ingest-6d8c7b5f9-m2kzp", "", "FailedMount", "MountVolume.SetUp failed for volume \"cache\"", 3, 5 * minute},
			{"shop", "Pod", "payments-api-5f7b8c9d4-tq8vn", "spec.containers{api}", "Unhealthy", "Liveness probe failed: context deadline exceeded", 2, 3 * minute},
		},
		svcs: []tSvc{
			{"kube-system", "traefik", corev1.ServiceTypeLoadBalancer, false},
			{"shop", "payments-lb", corev1.ServiceTypeLoadBalancer, true},
			{"shop", "payments-api", corev1.ServiceTypeClusterIP, false},
			{"default", "kubernetes", corev1.ServiceTypeClusterIP, false},
		},
		pvcs:  []tPVC{{"monitoring", "data-loki-backend-0", "Bound"}, {"shop", "data-postgres-0", "Bound"}, {"staging", "data-postgres-0", "Pending"}},
		helm:  []tHelm{{"traefik", "helm-install-traefik"}, {"traefik-crd", "helm-install-traefik-crd"}},
		snaps: []tSnap{{"k3s-cp-1", 10 * hour}, {"k3s-cp-1", 4 * hour}},
		podUse: []tPodUse{
			{"monitoring", "prometheus-k8s-0", "1420m", "3172Mi"}, {"shop", "payments-api-5f7b8c9d4-tq8vn", "910m", "612Mi"},
			{"growlytics", "ingest-6d8c7b5f9-m2kzp", "770m", "1434Mi"}, {"kube-system", "traefik-6f5f87584-x7k2p", "310m", "98Mi"},
			{"kube-system", "coredns-ccb96694c-8xq2z", "20m", "41Mi"},
		},
	}
}

// allClearK3s is healthy, with the probe warnings a real k3s keeps after
// boot: recovered readiness probes, an old helm-install back-off, a
// nameserver warning and a probe on a pod that is gone. It must read
// "all clear".
func allClearK3s(now time.Time) cluster {
	sys, sysPods := k3sSystem()
	pods := append(sysPods,
		run("kube-system", "traefik-6f5f87584-x7k2p", "k3s-cp-1", 12*day),
		tPod{ns: "kube-system", name: "helm-install-traefik-crd-8x2kq", node: "k3s-cp-1", status: "Completed", of: 1, age: 212 * day},
		run("shop", "payments-api-5f7b8c9d4-tq8vn", "k3s-agent-1", 3*day),
		run("growlytics", "web-7c9d8f6b5-p2r7x", "k3s-agent-2", 3*day),
	)
	old := run("growlytics", "ingest-6d8c7b5f9-m2kzp", "k3s-agent-2", 9*day)
	old.restarts, old.lastRestart = 2, 3*day
	pods = append(pods, old)
	return cluster{
		now: now, git: k3sGit, user: "devops-friend", metrics: true, readyz: readyOK,
		nodes: k3sNodes("3600Mi"), namespaces: k3sNamespaces()[:7], pods: pods,
		workloads: append(sys, tWorkload{"deployment", "shop", "payments-api", 1, 1}, tWorkload{"deployment", "growlytics", "web", 1, 1},
			tWorkload{"deployment", "growlytics", "ingest", 1, 1}),
		events: []tEvent{
			{"kube-system", "Pod", "metrics-server-5985cbc9d7-9w4kq", "spec.containers{metrics-server}", "Unhealthy",
				"Readiness probe failed: HTTP probe failed with statuscode: 500", 3, 52 * minute},
			{"kube-system", "Pod", "traefik-6f5f87584-x7k2p", "spec.containers{traefik}", "Unhealthy",
				`Readiness probe failed: Get "http://10.42.0.5:9000/ping": dial tcp 10.42.0.5:9000: connect: connection refused`, 2, 4 * minute},
			{"kube-system", "Pod", "coredns-ccb96694c-8xq2z", "spec.containers{coredns}", "Unhealthy",
				"Readiness probe failed: HTTP probe failed with statuscode: 503", 1, 2 * minute},
			{"kube-system", "Pod", "helm-install-traefik-crd-8x2kq", "spec.containers{helm}", "BackOff",
				"Back-off restarting failed container helm in pod helm-install-traefik-crd-8x2kq", 4, 38 * minute},
			{"kube-system", "Pod", "svclb-traefik-4a1b2c3d-abcde", "", "DNSConfigForming",
				"Nameserver limits were exceeded, some nameservers have been omitted, the applied nameserver line is: 10.0.0.2 1.1.1.1 8.8.8.8", 20, minute},
			{"kube-system", "Pod", "local-path-provisioner-5cf85fd84d-old01", "spec.containers{local-path-provisioner}", "Unhealthy",
				"Liveness probe failed: Get \"http://10.42.0.9:8080/healthz\": context deadline exceeded", 2, 6 * minute},
		},
		svcs:  []tSvc{{"kube-system", "traefik", corev1.ServiceTypeLoadBalancer, false}, {"default", "kubernetes", corev1.ServiceTypeClusterIP, false}},
		pvcs:  []tPVC{{"shop", "data-postgres-0", "Bound"}},
		helm:  []tHelm{{"traefik", "helm-install-traefik"}, {"traefik-crd", "helm-install-traefik-crd"}},
		snaps: []tSnap{{"k3s-cp-1", 4 * hour}},
	}
}

// noMetricsK3s is k3s started with --disable=metrics-server and without
// the etcdsnapshotfiles API (kine/SQLite has no etcd role).
func noMetricsK3s(now time.Time) cluster {
	c := allClearK3s(now)
	c.metrics = false
	c.snaps = nil
	c.workloads = c.workloads[:0:0]
	sys, _ := k3sSystem()
	for _, w := range sys {
		if w.name != "metrics-server" {
			c.workloads = append(c.workloads, w)
		}
	}
	nodes := k3sNodes("")
	nodes[0].roles = []string{"control-plane", "master"}
	c.nodes = nodes
	c.events = nil
	return c
}

// vanillaKind is upstream Kubernetes on kind: no metrics, no k3s APIs and
// a LoadBalancer nobody serves.
func vanillaKind(now time.Time) cluster {
	return cluster{
		now: now, git: "v1.33.1", user: "kubernetes-admin", readyz: readyOK,
		nodes:      []tNode{{name: "kind-control-plane", roles: []string{"control-plane"}, cpu: "8", mem: "16Gi", pods: 110, age: 2 * day}},
		namespaces: []tNS{{"kube-system", "Active", 2 * day}, {"default", "Active", 2 * day}},
		pods: []tPod{
			run("kube-system", "coredns-668d6bf9bc-abcde", "kind-control-plane", 2*day),
			run("default", "web-6d8c7b5f9-zz9zz", "kind-control-plane", day),
		},
		workloads: []tWorkload{{"deployment", "kube-system", "coredns", 1, 1}, {"deployment", "default", "web", 1, 1}},
		svcs:      []tSvc{{"default", "web-lb", corev1.ServiceTypeLoadBalancer, true}, {"default", "kubernetes", corev1.ServiceTypeClusterIP, false}},
	}
}

// nsScopedEKS is a namespace-scoped user on EKS without metrics-server.
func nsScopedEKS(now time.Time) cluster {
	pods := make([]tPod, 0, 6)
	for _, n := range []string{"api-1", "api-2", "web-1", "web-2", "worker-1", "cache-0"} {
		pods = append(pods, run("shop", n, "ip-10-0-1-23.ec2.internal", 4*day))
	}
	return cluster{
		now: now, git: "v1.35.4-eks-3025e55", user: "shop-dev", readyz: readyOK,
		nodes:      []tNode{{name: "ip-10-0-1-23.ec2.internal", cpu: "4", mem: "16Gi", pods: 58, age: 20 * day}},
		namespaces: []tNS{{"shop", "Active", 90 * day}, {"kube-system", "Active", 200 * day}},
		pods:       append(pods, run("other", "secret-app", "ip-10-0-1-23.ec2.internal", day)),
		workloads:  []tWorkload{{"deployment", "shop", "api", 2, 2}, {"deployment", "shop", "web", 2, 2}, {"deployment", "other", "app", 1, 1}},
		svcs:       []tSvc{{"shop", "api", corev1.ServiceTypeClusterIP, false}},
		pvcs:       []tPVC{{"shop", "data-cache-0", "Bound"}},
	}
}
