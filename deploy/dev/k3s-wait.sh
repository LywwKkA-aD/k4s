#!/bin/sh
# Waits for the k3s container to write a kubeconfig for its current start,
# installs it at <out> with a localhost server URL, and returns once the API
# answers with it.
#
# A kubeconfig left over from an earlier cluster passes a plain existence
# check but carries the old CA (x509: certificate signed by unknown
# authority), so it only counts once its mtime is at or after the container's
# start, and the copy is redone until /readyz succeeds.
#
# usage: k3s-wait.sh <container> <k3s kubeconfig> <out kubeconfig> [timeout seconds]
set -eu

if [ $# -lt 3 ]; then
  echo "usage: $0 <container> <k3s kubeconfig> <out kubeconfig> [timeout seconds]" >&2
  exit 2
fi
container=$1
src=$2
out=$3
timeout=${4:-180}

command -v kubectl >/dev/null || { echo "kubectl not on PATH" >&2; exit 1; }
docker inspect "$container" >/dev/null 2>&1 || {
  echo "container $container not found: run make k3s-up" >&2
  exit 1
}

# docker inspect prints RFC 3339 with nanoseconds; second precision is enough.
epoch_of() {
  ts=${1%%.*}
  ts=${ts%Z}
  date -u -d "$ts" +%s 2>/dev/null || date -u -j -f '%Y-%m-%dT%H:%M:%S' "$ts" +%s
}

mtime_of() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1"
}

started_at() {
  epoch_of "$(docker inspect -f '{{.State.StartedAt}}' "$container")"
}

tmp="$out.tmp.$$"
trap 'rm -f "$tmp"' EXIT

install_kubeconfig() {
  (umask 077 && sed 's#server: https://[^:]*:6443#server: https://127.0.0.1:6443#g' "$src" >"$tmp")
  mv "$tmp" "$out"
}

mkdir -p "$(dirname "$out")"
deadline=$(($(date +%s) + timeout))
echo "waiting for k3s ($container) to write a fresh kubeconfig and serve the API..."
while :; do
  if [ -f "$src" ] && [ "$(mtime_of "$src")" -ge "$(started_at)" ]; then
    install_kubeconfig
    if kubectl --kubeconfig "$out" get --raw=/readyz >/dev/null 2>&1; then
      echo "kubeconfig written to $out"
      echo "  export KUBECONFIG=$out"
      exit 0
    fi
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "k3s not ready after ${timeout}s; check: docker logs $container" >&2
    exit 1
  fi
  sleep 2
done
