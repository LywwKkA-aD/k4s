package cluster

// metricsInstall is the upstream install command for metrics-server.
const metricsInstall = "install metrics-server: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml"

// MetricsHint is what to tell the user when metrics.k8s.io is missing on
// d. k3s and rke2 bundle metrics-server, so a missing one was switched off
// at install time; minikube ships it as an addon; elsewhere it has to be
// installed.
func MetricsHint(d Distro) string {
	switch {
	case d.IsK3s():
		return "metrics-server is bundled with k3s; disabled with --disable=metrics-server?"
	case d == RKE2:
		return "metrics-server is bundled with rke2; disabled with --disable=rke2-metrics-server?"
	case d == Minikube:
		return "enable metrics-server: minikube addons enable metrics-server"
	}
	return metricsInstall
}
