#!/bin/sh
# Writes a kubeconfig that reaches the dev cluster as a namespaced
# ServiceAccount, with a 24h token from the TokenRequest API.
#
# modes:
#   token       the token sits in the kubeconfig (users[].user.token)
#   plugin      exec-plugin.sh hands the token over, interactiveMode Never
#   plugin-tty  exec-plugin.sh --require-tty, interactiveMode IfAvailable
#
# The plugin modes keep the token in <out>.token (0600) and point the
# kubeconfig at this checkout's exec-plugin.sh by absolute path.
#
# usage: kubeconfig-sa.sh <admin kubeconfig> <namespace> <serviceaccount> <out> [mode]
set -eu

if [ $# -lt 4 ]; then
  echo "usage: $0 <admin kubeconfig> <namespace> <serviceaccount> <out> [token|plugin|plugin-tty]" >&2
  exit 2
fi
admin=$1
ns=$2
sa=$3
out=$4
mode=${5:-token}

case $mode in
  token | plugin | plugin-tty) ;;
  *)
    echo "unknown mode $mode (want token, plugin, plugin-tty)" >&2
    exit 2
    ;;
esac
command -v kubectl >/dev/null || { echo "kubectl not on PATH" >&2; exit 1; }
if [ ! -f "$admin" ]; then
  echo "$admin not found: run make k3s-up first" >&2
  exit 1
fi

plugin="$(cd "$(dirname "$0")" && pwd)/exec-plugin.sh"
cluster=k4s-dev
user="$sa"
[ "$mode" = token ] || user="$sa-$mode"

server=$(kubectl --kubeconfig "$admin" config view --minify -o jsonpath='{.clusters[0].cluster.server}')
ca=$(kubectl --kubeconfig "$admin" config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
if ! kubectl --kubeconfig "$admin" -n "$ns" get serviceaccount "$sa" >/dev/null 2>&1; then
  echo "serviceaccount $ns/$sa not found: run make seed first" >&2
  exit 1
fi
token=$(kubectl --kubeconfig "$admin" -n "$ns" create token "$sa" --duration=24h)

umask 077
mkdir -p "$(dirname "$out")"

user_block() {
  case $mode in
    token)
      printf '      token: %s\n' "$token"
      ;;
    plugin | plugin-tty)
      interactive=Never
      extra=
      if [ "$mode" = plugin-tty ]; then
        interactive=IfAvailable
        extra=', "--require-tty"'
      fi
      printf '      exec:\n'
      printf '        apiVersion: client.authentication.k8s.io/v1\n'
      printf '        command: "%s"\n' "$plugin"
      printf '        args: ["--token-file", "%s"%s]\n' "$out.token" "$extra"
      printf '        interactiveMode: %s\n' "$interactive"
      printf '        provideClusterInfo: false\n'
      ;;
  esac
}

if [ "$mode" != token ]; then
  printf '%s\n' "$token" >"$out.token"
fi

{
  printf 'apiVersion: v1\n'
  printf 'kind: Config\n'
  printf 'clusters:\n'
  printf '  - name: %s\n' "$cluster"
  printf '    cluster:\n'
  printf '      server: %s\n' "$server"
  printf '      certificate-authority-data: %s\n' "$ca"
  printf 'users:\n'
  printf '  - name: %s\n' "$user"
  printf '    user:\n'
  user_block
  printf 'contexts:\n'
  printf '  - name: %s@%s\n' "$user" "$cluster"
  printf '    context:\n'
  printf '      cluster: %s\n' "$cluster"
  printf '      user: %s\n' "$user"
  printf '      namespace: %s\n' "$ns"
  printf 'current-context: %s@%s\n' "$user" "$cluster"
} >"$out"

echo "kubeconfig for $ns/$sa ($mode) written to $out"
echo "  export KUBECONFIG=$out"
