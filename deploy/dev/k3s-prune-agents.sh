#!/bin/sh
# Deletes the Node objects of k4s agents whose containers are not running.
#
# Stopping the agents profile leaves k4s-agent-* registered as NotReady, and
# the scheduler keeps placing pods on them (metrics-server stays Pending), so
# a single-node start would never become Ready. Deleting the Node also drops
# its node-password secret, so the agent rejoins cleanly when it comes back.
#
# usage: k3s-prune-agents.sh <kubeconfig>
set -eu

if [ $# -lt 1 ]; then
  echo "usage: $0 <kubeconfig>" >&2
  exit 2
fi
export KUBECONFIG="$1"

for node in $(kubectl get nodes -o name | sed -n 's#^node/\(k4s-agent-.*\)$#\1#p'); do
  # docker-compose.yml names each agent container after its hostname.
  if [ "$(docker inspect -f '{{.State.Running}}' "$node" 2>/dev/null || true)" != true ]; then
    kubectl delete node "$node"
  fi
done
