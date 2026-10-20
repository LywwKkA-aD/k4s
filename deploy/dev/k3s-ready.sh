#!/bin/sh
# Waits until every running node and every bundled k3s component that is not
# disabled are Ready: coredns, local-path, metrics-server (with node metrics
# served), traefik, and ServiceLB (every LoadBalancer Service has an address
# and its svclb DaemonSet is rolled out). It re-applies traefik when k3s
# lost it on a switch back from the minimal profile (see traefik_ready).
#
# It reads the profile from the running containers, not from make variables,
# so it waits for what is actually deployed: one node per running container
# of the compose project, and the disable list of the k3s config file the
# server was started with.
#
# usage: k3s-ready.sh <kubeconfig> <server container> [timeout seconds]
set -eu

if [ $# -lt 2 ]; then
  echo "usage: $0 <kubeconfig> <server container> [timeout seconds]" >&2
  exit 2
fi
export KUBECONFIG="$1"
container=$2
timeout=${3:-300}

command -v kubectl >/dev/null || { echo "kubectl not on PATH" >&2; exit 1; }
project=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$container" 2>/dev/null) || {
  echo "container $container not found: run make k3s-up" >&2
  exit 1
}
nodes=$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter status=running | wc -l | tr -d ' ')
# shellcheck disable=SC2016 # K3S_CONFIG_FILE is expanded inside the container
config=$(docker exec "$container" sh -c 'cat "${K3S_CONFIG_FILE:-/dev/null}"' 2>/dev/null || true)
disabled=",$(printf '%s\n' "$config" | sed -n 's/^disable: *\[\(.*\)\]/\1/p' | tr -d ' '),"

deadline=$(($(date +%s) + timeout))
# retry silences the commands it polls; fd 3 lets them still print a notice.
exec 3>&1

enabled() {
  case $disabled in
    *",$1,"*) return 1 ;;
    *) return 0 ;;
  esac
}

remaining() {
  left=$((deadline - $(date +%s)))
  [ "$left" -gt 1 ] || left=1
  echo "${left}s"
}

# retry <description> <command...>: polls until the command succeeds.
retry() {
  what=$1
  shift
  until "$@" >/dev/null 2>&1; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "timed out waiting for $what" >&2
      exit 1
    fi
    sleep 2
  done
  echo "  ready: $what"
}

nodes_ready() {
  count=$(kubectl get nodes -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' | grep -c '^True$' || true)
  [ "$count" -ge "$nodes" ]
}

# rolled_out <kind/name>: waits for a kube-system rollout, naming it on timeout.
rolled_out() {
  if ! kubectl -n kube-system rollout status "$1" --timeout="$(remaining)" >/dev/null; then
    echo "timed out waiting for $1; check: kubectl -n kube-system get pods -o wide" >&2
    exit 1
  fi
  echo "  ready: $1"
}

rollout() {
  retry "deployment/$1 exists" kubectl -n kube-system get deployment "$1"
  rolled_out "deployment/$1"
}

# The minimal profile makes k3s delete the traefik HelmChart. If that deletion
# is still finalizing when the default profile starts, the re-apply is lost
# while the Addon records traefik.yaml as applied, and traefik never returns.
# Dropping the Addon and touching the manifest makes k3s apply it again.
traefik_ready() {
  # "traefik/" when the chart is live, "traefik/<time>" while it is deleted.
  chart=$(kubectl -n kube-system get helmchart traefik -o jsonpath='{.metadata.name}/{.metadata.deletionTimestamp}' 2>/dev/null || true)
  if [ "$chart" = "traefik/" ] &&
    kubectl -n kube-system rollout status deployment traefik --timeout=5s >/dev/null 2>&1; then
    return 0
  fi
  if [ -z "$chart" ] && kubectl -n kube-system get addon traefik >/dev/null 2>&1; then
    echo "  re-applying traefik: k3s lost its HelmChart after a minimal profile" >&3
    kubectl -n kube-system delete addon traefik >/dev/null 2>&1 || true
    docker exec "$container" touch /var/lib/rancher/k3s/server/manifests/traefik.yaml || true
  fi
  return 1
}

# Fails when kubectl fails, so an API blip is never read as "all served".
lb_services_served() {
  out=$(kubectl get services -A -o jsonpath='{range .items[?(@.spec.type=="LoadBalancer")]}{.metadata.namespace}/{.metadata.name}={.status.loadBalancer.ingress[0].ip}{"\n"}{end}') || return 1
  ! printf '%s\n' "$out" | grep -q '=$'
}

traefik_svclb_exists() {
  [ -n "$(kubectl -n kube-system get daemonsets -l svccontroller.k3s.cattle.io/svcname=traefik -o name)" ]
}

svclb_rollout() {
  for ds in $(kubectl -n kube-system get daemonsets -l svccontroller.k3s.cattle.io/svcname -o name); do
    rolled_out "$ds"
  done
}

echo "waiting for $nodes node(s) and the bundled components..."
retry "$nodes node(s) Ready" nodes_ready
rollout coredns
if enabled local-storage; then rollout local-path-provisioner; fi
if enabled metrics-server; then
  rollout metrics-server
  retry "apiservice/v1beta1.metrics.k8s.io" \
    kubectl wait --for=condition=Available apiservice/v1beta1.metrics.k8s.io --timeout=5s
  retry "node metrics (kubectl top nodes)" kubectl top nodes
fi
if enabled traefik; then retry "deployment/traefik" traefik_ready; fi
if enabled servicelb; then
  if enabled traefik; then
    retry "svclb DaemonSet for traefik" traefik_svclb_exists
  fi
  retry "LoadBalancer Services have an address" lb_services_served
  svclb_rollout
fi
echo "cluster ready"
