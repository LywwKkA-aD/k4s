#!/bin/sh
# Applies the seed manifests: the namespaces first, then everything once each
# namespace has its default ServiceAccount. On a cluster that has just come up
# the controller-manager creates it a few seconds after the namespace, and
# until then bare Pods are rejected ("serviceaccount default not found").
#
# usage: seed.sh <kubeconfig> <seed dir> [timeout seconds]
set -eu

if [ $# -lt 2 ]; then
  echo "usage: $0 <kubeconfig> <seed dir> [timeout seconds]" >&2
  exit 2
fi
export KUBECONFIG="$1"
dir=$2
timeout=${3:-60}

command -v kubectl >/dev/null || { echo "kubectl not on PATH" >&2; exit 1; }

deadline=$(($(date +%s) + timeout))
for ns in $(kubectl apply -f "$dir/00-namespaces.yaml" -o name | sed -n 's#^namespace/##p'); do
  until kubectl -n "$ns" get serviceaccount default >/dev/null 2>&1; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "no default ServiceAccount in $ns after ${timeout}s" >&2
      exit 1
    fi
    sleep 1
  done
done
kubectl apply -f "$dir"
