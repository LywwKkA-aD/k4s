package k8s

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// The describe fixtures copy pods of the dev cluster's k4s-broken and
// k4s-demo seeds, at fixed times so the goldens hold.
var (
	describeNow = time.Date(2026, 10, 21, 14, 21, 0, 0, time.UTC)
	seededAt    = time.Date(2026, 10, 20, 22, 41, 45, 0, time.UTC)
	rebootedAt  = time.Date(2026, 10, 20, 22, 52, 44, 0, time.UTC)
)

const (
	busybox     = "busybox:1.37"
	busyboxID   = "docker.io/library/busybox@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e"
	saMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
)

func when(t time.Time) metav1.Time { return metav1.NewTime(t) }

func before(d time.Duration) metav1.Time { return when(describeNow.Add(-d)) }

func ptr[T any](v T) *T { return &v }

func smallResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("32Mi"),
			corev1.ResourceCPU:    resource.MustParse("50m"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("8Mi"),
			corev1.ResourceCPU:    resource.MustParse("10m"),
		},
	}
}

func tokenMount(volume string) corev1.VolumeMount {
	return corev1.VolumeMount{Name: volume, MountPath: saMountPath, ReadOnly: true}
}

// tokenVolume is the service account volume the API server adds to a pod.
func tokenVolume(name string) corev1.Volume {
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{
		Sources: []corev1.VolumeProjection{
			{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{ExpirationSeconds: ptr[int64](3607), Path: "token"}},
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"}}},
			{DownwardAPI: &corev1.DownwardAPIProjection{}},
		},
	}}}
}

func defaultTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr[int64](300)},
		{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: ptr[int64](300)},
	}
}

// seededPod is the part every fixture pod shares.
func seededPod(ns, name string, uid types.UID) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns, UID: uid,
			CreationTimestamp: when(seededAt),
			Labels:            map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{
			Priority:           ptr[int32](0),
			ServiceAccountName: "default",
			NodeName:           "k4s-server",
			Tolerations:        defaultTolerations(),
		},
		Status: corev1.PodStatus{
			HostIP:    "192.168.156.2",
			StartTime: ptr(when(seededAt)),
			QOSClass:  corev1.PodQOSBurstable,
		},
	}
}

func conditions(initialized, ready corev1.ConditionStatus) []corev1.PodCondition {
	return []corev1.PodCondition{
		{Type: "PodReadyToStartContainers", Status: corev1.ConditionTrue},
		{Type: corev1.PodInitialized, Status: initialized},
		{Type: corev1.PodReady, Status: ready},
		{Type: corev1.ContainersReady, Status: ready},
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
	}
}

func endedState(reason string, code int32, started, finished time.Time) *corev1.ContainerStateTerminated {
	return &corev1.ContainerStateTerminated{Reason: reason, ExitCode: code, StartedAt: when(started), FinishedAt: when(finished)}
}

func shell(script string) ([]string, []string) {
	return []string{"/bin/sh", "-c"}, []string{script}
}

// crashLoopPod waits in CrashLoopBackOff after its last run exited 1.
func crashLoopPod() *corev1.Pod {
	pod := seededPod("k4s-broken", "crash-loop", "d7c12e4a-8765-4b94-965d-04a713a91b58")
	cmd, args := shell("echo \"starting up...\"\nsleep 5\nexit 1\n")
	pod.Spec.Containers = []corev1.Container{{
		Name: "crasher", Image: busybox, Command: cmd, Args: args,
		Resources:    smallResources(),
		VolumeMounts: []corev1.VolumeMount{tokenMount("kube-api-access-92sxh")},
	}}
	pod.Spec.Volumes = []corev1.Volume{tokenVolume("kube-api-access-92sxh")}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "10.42.0.27"
	pod.Status.PodIPs = []corev1.PodIP{{IP: "10.42.0.27"}}
	pod.Status.Conditions = conditions(corev1.ConditionTrue, corev1.ConditionFalse)
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:                 "crasher",
		ContainerID:          "containerd://1084858349c6",
		Image:                busybox,
		ImageID:              busyboxID,
		State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off 5m0s restarting failed container"}},
		LastTerminationState: corev1.ContainerState{Terminated: endedState("Error", 1, describeNow.Add(-5*time.Minute), describeNow.Add(-5*time.Minute+5*time.Second))},
		RestartCount:         78,
	}}
	return pod
}

// initFailurePod is stuck on its second init container, which keeps
// exiting 1, so the app container never starts.
func initFailurePod() *corev1.Pod {
	pod := seededPod("k4s-broken", "init-crashloop", "9f128041-0113-4332-b8fc-95bb146bbd47")
	checkCmd, checkArgs := shell("echo \"config present\"")
	migrateCmd, migrateArgs := shell("echo \"running schema migrations...\"\nexit 1\n")
	pod.Spec.InitContainers = []corev1.Container{
		{Name: "check-config", Image: busybox, Command: checkCmd, Args: checkArgs, Resources: smallResources(), VolumeMounts: []corev1.VolumeMount{tokenMount("kube-api-access-4x7zv")}},
		{Name: "migrate", Image: busybox, Command: migrateCmd, Args: migrateArgs, Resources: smallResources(), VolumeMounts: []corev1.VolumeMount{tokenMount("kube-api-access-4x7zv")}},
	}
	pod.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "nginx:1.27-alpine",
		Ports:        []corev1.ContainerPort{{ContainerPort: 80, Protocol: corev1.ProtocolTCP}},
		VolumeMounts: []corev1.VolumeMount{tokenMount("kube-api-access-4x7zv")},
	}}
	pod.Spec.Volumes = []corev1.Volume{tokenVolume("kube-api-access-4x7zv")}
	pod.Status.Phase = corev1.PodPending
	pod.Status.PodIP = "10.42.0.39"
	pod.Status.PodIPs = []corev1.PodIP{{IP: "10.42.0.39"}}
	pod.Status.Conditions = conditions(corev1.ConditionFalse, corev1.ConditionFalse)
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
		{
			Name: "check-config", ContainerID: "containerd://6c3b329240d9", Image: busybox, ImageID: busyboxID,
			State:                corev1.ContainerState{Terminated: endedState("Completed", 0, rebootedAt.Add(8*time.Second), rebootedAt.Add(8*time.Second))},
			LastTerminationState: corev1.ContainerState{Terminated: endedState("Completed", 0, seededAt.Add(9*time.Second), seededAt.Add(9*time.Second))},
			Ready:                true,
			RestartCount:         1,
		},
		{
			Name: "migrate", ContainerID: "containerd://09a84f36b07b", Image: busybox, ImageID: busyboxID,
			State:                corev1.ContainerState{Terminated: endedState("Error", 1, describeNow.Add(-40*time.Second), describeNow.Add(-37*time.Second))},
			LastTerminationState: corev1.ContainerState{Terminated: endedState("Error", 1, describeNow.Add(-6*time.Minute), describeNow.Add(-6*time.Minute+3*time.Second))},
			RestartCount:         73,
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app", Image: "nginx:1.27-alpine",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}},
	}}
	return pod
}

// sidecarPod runs a native sidecar: an init container that restarts
// always and keeps running next to the app. Its ReplicaSet controls it.
func sidecarPod() *corev1.Pod {
	pod := seededPod("k4s-demo", "native-sidecar", "4a1d3b77-26c9-4f0e-9a35-5b8f2d6c1e90")
	pod.OwnerReferences = []metav1.OwnerReference{
		{APIVersion: "v1", Kind: "ConfigMap", Name: "not-the-controller"},
		{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "native-sidecar-6b7f9", Controller: ptr(true)},
	}
	pod.Spec.Priority = ptr[int32](1000)
	pod.Spec.PriorityClassName = "workload-high"
	pod.Spec.ServiceAccountName = "shipper"
	logs := corev1.VolumeMount{Name: "logs", MountPath: "/var/log/app"}
	shipCmd, shipArgs := shell("tail -F /var/log/app/app.log")
	setupCmd, setupArgs := shell("echo setup done")
	appCmd, appArgs := shell("while true; do sleep 2; done")
	pod.Spec.InitContainers = []corev1.Container{
		{
			Name: "log-shipper", Image: busybox, Command: shipCmd, Args: shipArgs,
			RestartPolicy: ptr(corev1.ContainerRestartPolicyAlways),
			Resources:     smallResources(),
			StartupProbe: &corev1.Probe{
				ProbeHandler:   corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"test", "-f", "/var/log/app/app.log"}}},
				TimeoutSeconds: 1, PeriodSeconds: 1, SuccessThreshold: 1, FailureThreshold: 30,
			},
			VolumeMounts: []corev1.VolumeMount{tokenMount("kube-api-access-gdslw"), logs},
		},
		{Name: "setup", Image: busybox, Command: setupCmd, Args: setupArgs, VolumeMounts: []corev1.VolumeMount{logs}},
	}
	pod.Spec.Containers = []corev1.Container{{
		Name: "app", Image: busybox, Command: appCmd, Args: appArgs,
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
			{Name: "metrics", ContainerPort: 9090, HostPort: 9090, Protocol: corev1.ProtocolTCP},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler:  corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("http")}},
			PeriodSeconds: 10, SuccessThreshold: 1, FailureThreshold: 3, TimeoutSeconds: 1,
		},
		EnvFrom: []corev1.EnvFromSource{
			{Prefix: "APP_", ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-env"}}},
			{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-secrets"}, Optional: ptr(true)}},
		},
		Env: []corev1.EnvVar{
			{Name: "LOG_DIR", Value: "/var/log/app"},
			{Name: "BANNER", Value: "two\nlines"},
			{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"}}},
			{Name: "TIER", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.labels['app']"}}},
			{Name: "API_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "api"}, Key: "token"}}},
			{Name: "MODE", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}, Key: "mode", Optional: ptr(true)}}},
			{Name: "CPU_LIMIT", ValueFrom: &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.cpu"}}},
			{Name: "MEM_REQUEST_MI", ValueFrom: &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "requests.memory", Divisor: resource.MustParse("1Mi")}}},
		},
		Resources:    corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("48Mi")}},
		VolumeMounts: []corev1.VolumeMount{logs, {Name: "config", MountPath: "/etc/app", SubPath: "app.conf", ReadOnly: true}},
	}}
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "logs", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}}},
		tokenVolume("kube-api-access-gdslw"),
	}
	pod.Spec.NodeSelector = map[string]string{"kubernetes.io/os": "linux"}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "10.42.0.38"
	pod.Status.PodIPs = []corev1.PodIP{{IP: "10.42.0.38"}, {IP: "fd00:10:42::26"}}
	pod.Status.Conditions = conditions(corev1.ConditionTrue, corev1.ConditionTrue)
	lostAtReboot := corev1.ContainerState{Terminated: endedState("Unknown", 255, seededAt.Add(9*time.Second), rebootedAt)}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
		{
			Name: "log-shipper", ContainerID: "containerd://44884348fc53", Image: busybox, ImageID: busyboxID,
			State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: when(rebootedAt.Add(8 * time.Second))}},
			LastTerminationState: lostAtReboot,
			Ready:                true, RestartCount: 1,
		},
		{
			Name: "setup", ContainerID: "containerd://1e6cbbb0b98b", Image: busybox, ImageID: busyboxID,
			State: corev1.ContainerState{Terminated: endedState("Completed", 0, rebootedAt.Add(9*time.Second), rebootedAt.Add(9*time.Second))},
			Ready: true, RestartCount: 1,
		},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app", ContainerID: "containerd://f93962b0e467", Image: busybox, ImageID: busyboxID,
		State:                corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: when(rebootedAt.Add(10 * time.Second))}},
		LastTerminationState: lostAtReboot,
		Ready:                true, RestartCount: 1,
	}}
	return pod
}

// podEvent is an event about pod, from the kubelet unless from is set.
func podEvent(pod *corev1.Pod, name, typ, reason, msg string, count int32, first, last metav1.Time) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name + "." + name, Namespace: pod.Namespace},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID, APIVersion: "v1",
		},
		Type: typ, Reason: reason, Message: msg, Count: count,
		FirstTimestamp: first, LastTimestamp: last,
		Source: corev1.EventSource{Component: "kubelet", Host: "k4s-server"},
	}
}

func crashLoopEvents(pod *corev1.Pod) []runtime.Object {
	earlier := pod.DeepCopy()
	earlier.UID = "0a0b0c0d-0000-4000-8000-000000000000"
	return []runtime.Object{
		podEvent(pod, "backoff", corev1.EventTypeWarning, "BackOff",
			"Back-off restarting failed container crasher in pod crash-loop_k4s-broken", 294, when(seededAt), before(51*time.Second)),
		podEvent(pod, "pulled", corev1.EventTypeNormal, "Pulled",
			`Container image "busybox:1.37" already present on machine`, 72, when(seededAt), before(4*time.Minute+45*time.Second)),
		podEvent(pod, "created", corev1.EventTypeNormal, "Created", "Container created\n", 62, when(seededAt), before(57*time.Minute)),
		// The pod of the same name that ran before this one.
		podEvent(earlier, "old", corev1.EventTypeWarning, "FailedMount", "an earlier pod's event", 1, before(48*time.Hour), before(48*time.Hour)),
	}
}

func initFailureEvents(pod *corev1.Pod) []runtime.Object {
	scheduled := podEvent(pod, "scheduled", corev1.EventTypeNormal, "Scheduled",
		"Successfully assigned k4s-broken/init-crashloop to k4s-server", 0, metav1.Time{}, metav1.Time{})
	scheduled.EventTime = metav1.NewMicroTime(seededAt)
	scheduled.Source = corev1.EventSource{}
	scheduled.ReportingController = "default-scheduler"
	backoff := podEvent(pod, "backoff", corev1.EventTypeWarning, "BackOff",
		"Back-off restarting failed container migrate in pod init-crashloop_k4s-broken", 0, metav1.Time{}, metav1.Time{})
	backoff.EventTime = metav1.NewMicroTime(seededAt)
	backoff.Series = &corev1.EventSeries{Count: 301, LastObservedTime: metav1.NewMicroTime(describeNow.Add(-111 * time.Second))}
	return []runtime.Object{scheduled, backoff}
}

func webDeployment() *appsv1.Deployment {
	maxUnavailable, maxSurge := intstr.FromString("25%"), intstr.FromString("25%")
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nginx", Namespace: "k4s-demo", UID: "2f9c0d61-7a3e-4b8e-9d0a-6c1f5e2b7a44",
			CreationTimestamp: when(seededAt),
			Labels:            map[string]string{"app": "nginx"},
			Annotations: map[string]string{
				"deployment.kubernetes.io/revision": "1",
				lastAppliedConfigKey:                `{"apiVersion":"apps/v1"}`,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr[int32](3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Strategy: appsv1.DeploymentStrategy{
				Type:          appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &maxUnavailable, MaxSurge: &maxSurge},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx", "tier": "front"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "nginx", Image: "nginx:1.27-alpine",
						Ports: []corev1.ContainerPort{{ContainerPort: 80, Protocol: corev1.ProtocolTCP}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromInt32(80), Scheme: corev1.URISchemeHTTP}},
							InitialDelaySeconds: 2, TimeoutSeconds: 1, PeriodSeconds: 5, SuccessThreshold: 1, FailureThreshold: 3,
						},
						Env: []corev1.EnvVar{{Name: "POD_IP", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"}}}},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi"), corev1.ResourceCPU: resource.MustParse("100m")},
						},
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 3, UpdatedReplicas: 3, ReadyReplicas: 2, AvailableReplicas: 2, UnavailableReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "MinimumReplicasAvailable"},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
			},
		},
	}
}

func webDeploymentEvents(d *appsv1.Deployment) []runtime.Object {
	return []runtime.Object{&corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "nginx.scaled", Namespace: d.Namespace},
		InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Namespace: d.Namespace, Name: d.Name, UID: d.UID},
		Type:           corev1.EventTypeNormal, Reason: "ScalingReplicaSet",
		Message: "Scaled up replica set nginx-59f9f966f from 0 to 3",
		Count:   1, FirstTimestamp: when(seededAt), LastTimestamp: when(seededAt),
		Source: corev1.EventSource{Component: "deployment-controller"},
	}}
}

func lbService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nginx-lb", Namespace: "k4s-demo", UID: "71c2a9e0-5d3b-4f6a-8e1c-0b9d4a7f2c35",
			CreationTimestamp: when(seededAt),
			Labels:            map[string]string{"app": "nginx"},
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              map[string]string{"app": "nginx"},
			IPFamilyPolicy:        ptr(corev1.IPFamilyPolicySingleStack),
			IPFamilies:            []corev1.IPFamily{corev1.IPv4Protocol},
			ClusterIP:             "10.43.12.7",
			ClusterIPs:            []string{"10.43.12.7"},
			SessionAffinity:       corev1.ServiceAffinityNone,
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyCluster,
			InternalTrafficPolicy: ptr(corev1.ServiceInternalTrafficPolicyCluster),
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(80), NodePort: 31234, Protocol: corev1.ProtocolTCP},
				{Port: 9090, TargetPort: intstr.FromString("metrics"), NodePort: 31999, Protocol: corev1.ProtocolTCP},
			},
		},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{IP: "192.168.156.2", IPMode: ptr(corev1.LoadBalancerIPModeVIP)}, {Hostname: "lb.example.test"}},
		}},
	}
}
