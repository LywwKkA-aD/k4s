package k8s

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// squash collapses each run of spaces to one, so a test reads a line of
// tabwriter output without counting its padding.
func squash(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

func TestDescribeEnvironmentAndProbes(t *testing.T) {
	t.Parallel()

	pod := sidecarPod()
	out, err := fakeClient(pod).describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
	if err != nil {
		t.Fatalf("describePod: %v", err)
	}
	got := squash(out)
	wants := []string{
		"Startup: exec [test -f /var/log/app/app.log] delay=0s timeout=1s period=1s #success=1 #failure=30\n",
		"Liveness: tcp-socket :http delay=0s timeout=1s period=10s #success=1 #failure=3\n",
		"Environment Variables from:\napp-env ConfigMap with prefix 'APP_' Optional: false\napp-secrets Secret Optional: true\n",
		"Environment:\nLOG_DIR: /var/log/app\nBANNER: two\nlines\n",
		"POD_NAME: native-sidecar (v1:metadata.name)\n",
		"TIER: native-sidecar (v1:metadata.labels['app'])\n",
		"API_TOKEN: <set to the key 'token' in secret 'api'> Optional: false\n",
		"MODE: <set to the key 'mode' of config map 'app-config'> Optional: true\n",
		"CPU_LIMIT: node allocatable (limits.cpu)\n",
		"MEM_REQUEST_MI: 48 (requests.memory)\n",
		"Environment: <none>\n",
		"TokenExpirationSeconds: 3607\nConfigMapName: kube-root-ca.crt\nOptional: false\nDownwardAPI: true\n",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("describe lacks %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "Restart Policy") {
		t.Errorf("kubectl describe shows no restart policy:\n%s", out)
	}
}

func TestDescribeTemplateProbeAndEnv(t *testing.T) {
	t.Parallel()

	d := webDeployment()
	out, err := fakeClient(d).describeDeployment(context.Background(), d.Namespace, d.Name, describeNow)
	if err != nil {
		t.Fatalf("describeDeployment: %v", err)
	}
	got := squash(out)
	for _, w := range []string{
		"Readiness: http-get http://:80/ delay=2s timeout=1s period=5s #success=1 #failure=3\n",
		// A template has no pod to read a field from.
		"POD_IP: (v1:status.podIP)\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("describe lacks %q:\n%s", w, out)
		}
	}
}

func TestProbeText(t *testing.T) {
	t.Parallel()

	attrs := "delay=0s timeout=0s period=0s #success=0 #failure=0"
	cases := []struct {
		name  string
		probe corev1.Probe
		want  string
	}{
		{
			name:  "https on a host",
			probe: corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Host: "10.0.0.1", Port: intstr.FromInt32(8443), Path: "/healthz", Scheme: corev1.URISchemeHTTPS}}},
			want:  "http-get https://10.0.0.1:8443/healthz " + attrs,
		},
		{
			name:  "grpc",
			probe: corev1.Probe{ProbeHandler: corev1.ProbeHandler{GRPC: &corev1.GRPCAction{Port: 9000, Service: ptr("health")}}},
			want:  "grpc <pod>:9000 health " + attrs,
		},
		{
			name:  "grpc without service",
			probe: corev1.Probe{ProbeHandler: corev1.ProbeHandler{GRPC: &corev1.GRPCAction{Port: 9000}}},
			want:  "grpc <pod>:9000  " + attrs,
		},
		{
			name:  "tcp on a host",
			probe: corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Host: "db", Port: intstr.FromInt32(5432)}}},
			want:  "tcp-socket db:5432 " + attrs,
		},
		{name: "unknown", probe: corev1.Probe{}, want: "unknown " + attrs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := probeText(&tc.probe); got != tc.want {
				t.Errorf("probeText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFieldValue(t *testing.T) {
	t.Parallel()

	pod := sidecarPod()
	pod.Annotations = map[string]string{"team": "core"}
	cases := map[string]string{
		"metadata.name":                "native-sidecar",
		"metadata.namespace":           "k4s-demo",
		"metadata.uid":                 string(pod.UID),
		"metadata.labels['app']":       "native-sidecar",
		"metadata.annotations['team']": "core",
		"metadata.labels['missing']":   "",
		"spec.nodeName":                "k4s-server",
		"spec.serviceAccountName":      "shipper",
		"status.hostIP":                "192.168.156.2",
		"status.podIP":                 "10.42.0.38",
		"status.podIPs":                "10.42.0.38,fd00:10:42::26",
		"spec.unknown":                 "",
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if got := fieldValue(pod, path); got != want {
				t.Errorf("fieldValue(%q) = %q, want %q", path, got, want)
			}
		})
	}
}
